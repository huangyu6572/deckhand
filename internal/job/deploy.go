package job

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"localaihub/internal/storage"
	"localaihub/internal/transport/contract"
	"localaihub/internal/wire"
)

type Recipe struct {
	Name     string       `yaml:"name"`
	Precheck []string     `yaml:"precheck"`
	Upload   RecipeUpload `yaml:"upload"`
	Apply    []string     `yaml:"apply"`
	Verify   RecipeVerify `yaml:"verify"`
	Rollback []string     `yaml:"rollback"`
}

type RecipeUpload struct {
	RemoteDir  string `yaml:"remote_dir"`
	RemoteName string `yaml:"remote_name"`
}

type RecipeVerify struct {
	Type    string `yaml:"type"`
	Command string `yaml:"command"`
	Timeout string `yaml:"timeout"`
}

func (s *Service) Deploy(ctx context.Context, requestID, target, recipeName, artifact, idem string, timeout time.Duration, allowPublic bool) (wire.Result, error) {
	t, err := s.Resolve(ctx, target, allowPublic)
	if err != nil {
		return nil, err
	}
	if t.Transport != "ssh" {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "deploy requires SSH")
	}
	path := filepath.Join(s.DataDir, "recipes", recipeName+".yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, wire.E("RECIPE_NOT_FOUND", path)
		}
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var rec Recipe
	if err := dec.Decode(&rec); err != nil {
		return nil, wire.Ef("RECIPE_INVALID", "%v", err)
	}
	if rec.Verify.Type != "" && rec.Verify.Type != "command" {
		return nil, wire.E("RECIPE_INVALID", "verify.type must be command")
	}
	sum := sha256.Sum256(b)
	recipeHash := hex.EncodeToString(sum[:])
	artHash, err := shaFile(artifact)
	if err != nil {
		return nil, wire.E("LOCAL_IO_ERROR", err.Error())
	}
	fp := recipeHash + ":" + artHash
	if idem != "" {
		oldFP, oldID, err := s.DB.IdempotencyGet(ctx, t.Identity, recipeName, idem)
		if err != nil {
			return nil, err
		}
		if oldID != "" {
			if oldFP != fp {
				return nil, wire.E("IDEMPOTENCY_CONFLICT", "same key different fingerprint")
			}
			op, err := s.DB.Get(ctx, oldID)
			if err != nil {
				return nil, err
			}
			if op.State == "succeeded" {
				return s.final(oldID, requestID), nil
			}
		}
	}
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	op, w, err := s.createOp(ctx, requestID, "deploy", t, map[string]any{"recipe": recipeName, "artifact": artifact})
	if err != nil {
		return nil, err
	}
	if idem != "" {
		_ = s.DB.IdempotencyPut(ctx, t.Identity, recipeName, idem, fp, op.ID)
	}
	_ = s.DB.PutDeployment(ctx, storage.Deployment{
		OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash,
		ArtifactSHA256: artHash, CurrentStep: "precheck", DeployStatus: "running",
	})
	jctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	now := storage.NowUS()
	_, _ = w.Append("started", "", map[string]any{"state": "running"})
	_ = s.DB.UpdateRunning(ctx, op.ID, op.Version, now, now, w.Cursor())
	op.Version++

	entry, err := s.Pool.Acquire(jctx, t)
	if err != nil {
		s.fail(op, w, err)
		return s.final(op.ID, requestID), nil
	}
	defer s.Pool.Release(entry)
	ex, ok := entry.Transport.(contract.ExecConn)
	fc, ok2 := entry.Transport.(contract.FileConn)
	if !ok || !ok2 {
		s.fail(op, w, wire.E("CAPABILITY_UNSUPPORTED", "deploy needs exec and files"))
		return s.final(op.ID, requestID), nil
	}

	runCmd := func(cmd string, stepTimeout time.Duration) (int, error) {
		cctx := jctx
		var ccancel context.CancelFunc
		if stepTimeout > 0 {
			cctx, ccancel = context.WithTimeout(jctx, stepTimeout)
			defer ccancel()
		}
		stdout := &chunkWriter{w: w, typ: "stdout"}
		stderr := &chunkWriter{w: w, typ: "stderr"}
		return ex.Exec(cctx, cmd, stdout, stderr)
	}

	setStep := func(step, st string) {
		_, _ = w.Append("state", "", map[string]any{"current_step": step, "deploy_status": st})
		_ = s.DB.PutDeployment(ctx, storage.Deployment{
			OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash,
			ArtifactSHA256: artHash, CurrentStep: step, DeployStatus: st,
		})
	}

	for _, c := range rec.Precheck {
		exit, err := runCmd(c, 0)
		if err != nil || exit != 0 {
			s.fail(op, w, wire.E("REMOTE_EXIT_NONZERO", "precheck failed"))
			_ = s.DB.PutDeployment(ctx, storage.Deployment{OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash, ArtifactSHA256: artHash, CurrentStep: "precheck", DeployStatus: "failed"})
			return s.final(op.ID, requestID), nil
		}
	}
	remote := filepath.ToSlash(filepath.Join(rec.Upload.RemoteDir, rec.Upload.RemoteName))
	setStep("upload", "running")
	if _, _, err := fc.Upload(jctx, artifact, remote); err != nil {
		s.fail(op, w, err)
		_ = s.DB.PutDeployment(ctx, storage.Deployment{OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash, ArtifactSHA256: artHash, CurrentStep: "upload", DeployStatus: "failed"})
		return s.final(op.ID, requestID), nil
	}
	setStep("apply", "running")
	for _, c := range rec.Apply {
		exit, err := runCmd(c, 0)
		if err != nil || exit != 0 {
			s.fail(op, w, wire.E("REMOTE_EXIT_NONZERO", "apply failed"))
			_ = s.DB.PutDeployment(ctx, storage.Deployment{OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash, ArtifactSHA256: artHash, CurrentStep: "apply", DeployStatus: "failed"})
			return s.final(op.ID, requestID), nil
		}
	}
	setStep("verify", "verifying")
	vtimeout := 30 * time.Second
	if rec.Verify.Timeout != "" {
		if d, err := time.ParseDuration(rec.Verify.Timeout); err == nil {
			vtimeout = d
		}
	}
	if rec.Verify.Command != "" {
		exit, err := runCmd(rec.Verify.Command, vtimeout)
		if err != nil || exit != 0 {
			setStep("rollback", "rolling_back")
			rbOK := true
			for _, c := range rec.Rollback {
				exit, err := runCmd(c, 0)
				if err != nil || exit != 0 {
					rbOK = false
					break
				}
			}
			if rbOK && len(rec.Rollback) > 0 {
				e := wire.E("HEALTHCHECK_FAILED", "verify failed")
				e.Status = "failed"
				s.fail(op, w, e)
				_ = s.DB.PutDeployment(ctx, storage.Deployment{OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash, ArtifactSHA256: artHash, CurrentStep: "rollback", DeployStatus: "rolled_back"})
			} else {
				e := wire.E("ROLLBACK_FAILED", "rollback failed")
				s.fail(op, w, e)
				_ = s.DB.PutDeployment(ctx, storage.Deployment{OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash, ArtifactSHA256: artHash, CurrentStep: "rollback", DeployStatus: "rollback_failed"})
			}
			return s.final(op.ID, requestID), nil
		}
	}
	zero := 0
	_, _ = w.Append("completed", "", map[string]any{"state": "succeeded", "deploy_status": "succeeded"})
	_ = s.DB.Finish(ctx, op.ID, op.Version, "succeeded", "", &zero, w.Cursor(), false)
	_ = s.DB.PutDeployment(ctx, storage.Deployment{OperationID: op.ID, RecipeName: recipeName, RecipeHash: recipeHash, ArtifactSHA256: artHash, CurrentStep: "verify", DeployStatus: "succeeded"})
	return s.final(op.ID, requestID), nil
}
