package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"localaihub/internal/config"
	"localaihub/internal/destructive"
	"localaihub/internal/log"
	"localaihub/internal/storage"
	"localaihub/internal/transport/contract"
	"localaihub/internal/transport/pool"
	"localaihub/internal/wire"
	"localaihub/internal/workspace"
)

type running struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Service struct {
	DataDir string
	DB      *storage.DB
	Logs    *logstore.Store
	Pool    *pool.Pool
	Limits  config.Limits
	Resolve func(ctx context.Context, ref string, allowPublic bool) (*config.Resolved, error)
	sem     chan struct{}
	mu      sync.Mutex
	live    map[string]*running
}

func New(dataDir string, db *storage.DB, logs *logstore.Store, p *pool.Pool, limits config.Limits, resolve func(context.Context, string, bool) (*config.Resolved, error)) *Service {
	n := limits.MaxConcurrentJobs
	if n <= 0 {
		n = 32
	}
	return &Service{
		DataDir: dataDir,
		DB:      db,
		Logs:    logs,
		Pool:    p,
		Limits:  limits,
		Resolve: resolve,
		sem:     make(chan struct{}, n),
		live:    map[string]*running{},
	}
}

type RunReq struct {
	Target       string
	Command      string
	Inspect      string
	Timeout      time.Duration
	AllowPublic  bool
	JSONL        bool
	Detach       bool
	Sensitive    bool
	RequestID    string
	RmConfirmed  bool
	Workdir      string
}

func (s *Service) Run(ctx context.Context, req RunReq, emit func(wire.Event), stream bool) (wire.Result, error) {
	if strings.TrimSpace(req.Command) == "" {
		return nil, wire.E("INVALID_ARGUMENT", "command is required")
	}
	inspect := req.Inspect
	if inspect == "" {
		inspect = req.Command
	}
	if err := destructive.RefuseUnlessConfirmed(inspect+"\n"+req.Command, req.RmConfirmed); err != nil {
		return nil, err
	}
	t, err := s.Resolve(ctx, req.Target, req.AllowPublic)
	if err != nil {
		return nil, err
	}
	if t.Transport != "ssh" {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "hub run requires SSH exec")
	}
	wd, err := workspace.Confine(req.Workdir, t.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if wd != "" {
		if err := workspace.CheckCommand(inspect, wd); err != nil {
			return nil, err
		}
		req.Command = workspace.Bind(inspect, wd)
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = t.DefaultTimeout
	}
	op, w, err := s.createOp(ctx, req.RequestID, "run", t, map[string]any{
		"command":   hide(req.Command, req.Sensitive),
		"sensitive": req.Sensitive,
	})
	if err != nil {
		return nil, err
	}
	if req.Detach {
		go s.execRun(context.Background(), op, w, t, req.Command, timeout, nil)
		return s.snapshot(op.ID, req.RequestID, true), nil
	}
	if stream && emit != nil {
		ch, unsub := w.Subscribe(64)
		defer unsub()
		go func() {
			for ev := range ch {
				emit(ev)
			}
		}()
	}
	s.execRun(ctx, op, w, t, req.Command, timeout, emit)
	return s.final(op.ID, req.RequestID), nil
}

func (s *Service) execRun(ctx context.Context, op *storage.Operation, w *logstore.Writer, t *config.Resolved, command string, timeout time.Duration, emit func(wire.Event)) {
	jctx, cancel := context.WithTimeout(ctx, timeout)
	s.track(op.ID, cancel)
	defer s.untrack(op.ID)
	defer cancel()

	now := storage.NowUS()
	_, _ = w.Append("started", "", map[string]any{"state": "running"})
	_ = s.DB.UpdateRunning(context.Background(), op.ID, op.Version, now, now, w.Cursor())
	op.Version++

	entry, err := s.Pool.Acquire(jctx, t)
	if err != nil {
		s.fail(op, w, err)
		return
	}
	defer s.Pool.Release(entry)
	ex, ok := entry.Transport.(contract.ExecConn)
	if !ok {
		s.fail(op, w, wire.E("CAPABILITY_UNSUPPORTED", "transport has no exec"))
		return
	}
	stdout := &chunkWriter{w: w, typ: "stdout"}
	stderr := &chunkWriter{w: w, typ: "stderr"}
	exit, err := ex.Exec(jctx, command, stdout, stderr)
	if err != nil {
		s.fail(op, w, err)
		return
	}
	state := "succeeded"
	code := ""
	if exit != 0 {
		state = "failed"
		code = "REMOTE_EXIT_NONZERO"
	}
	_, _ = w.Append("completed", "", map[string]any{"state": state, "exit_code": exit, "error_code": code})
	_ = s.DB.Finish(context.Background(), op.ID, op.Version, state, code, &exit, w.Cursor(), false)
}

func (s *Service) Copy(ctx context.Context, requestID, src, dst string, timeout time.Duration, allowPublic bool, workdir string) (wire.Result, error) {
	spec, err := parseCopy(src, dst)
	if err != nil {
		return nil, err
	}
	t, err := s.Resolve(ctx, spec.Target, allowPublic)
	if err != nil {
		return nil, err
	}
	if t.Transport != "ssh" {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "hub cp requires SSH/SFTP")
	}
	wd, err := workspace.Confine(workdir, t.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if wd != "" && !workspace.Under(spec.Remote, wd) {
		return nil, wire.E("FILE_OUTSIDE_WORKSPACE", "remote path outside --workdir / workspace_root")
	}
	if timeout <= 0 {
		timeout = t.DefaultTimeout
		if timeout < 10*time.Minute {
			timeout = 10 * time.Minute
		}
	}
	op, w, err := s.createOp(ctx, requestID, "cp", t, map[string]any{"src": src, "dst": dst, "direction": spec.Direction})
	if err != nil {
		return nil, err
	}
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
	fc, ok := entry.Transport.(contract.FileConn)
	if !ok {
		s.fail(op, w, wire.E("CAPABILITY_UNSUPPORTED", "no file capability"))
		return s.final(op.ID, requestID), nil
	}
	var n int64
	var sum string
	if spec.Direction == "upload" {
		n, sum, err = fc.Upload(jctx, spec.Local, spec.Remote)
	} else {
		n, sum, err = fc.Download(jctx, spec.Remote, spec.Local)
	}
	if err != nil {
		s.fail(op, w, err)
		return s.final(op.ID, requestID), nil
	}
	_, _ = w.Append("completed", "", map[string]any{"state": "succeeded", "bytes": n, "sha256": sum})
	zero := 0
	_ = s.DB.Finish(ctx, op.ID, op.Version, "succeeded", "", &zero, w.Cursor(), false)
	r := s.final(op.ID, requestID)
	r["direction"] = spec.Direction
	r["bytes"] = n
	r["sha256"] = sum
	r["remote_path"] = spec.Remote
	r["local_path"] = spec.Local
	return r, nil
}

func (s *Service) SerialExec(ctx context.Context, requestID, target, payload, wait string, timeout time.Duration, baud int, allowPublic bool) (wire.Result, error) {
	t, err := s.Resolve(ctx, target, allowPublic)
	if err != nil {
		return nil, err
	}
	if t.Transport != "serial" {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "serial exec requires a COM target")
	}
	if baud > 0 {
		t.BaudRate = baud
	}
	if timeout <= 0 {
		timeout = t.DefaultTimeout
	}
	if wait == "" {
		wait = t.PromptPattern
	}
	op, w, err := s.createOp(ctx, requestID, "serial_exec", t, map[string]any{"payload": payload})
	if err != nil {
		return nil, err
	}
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
	sc, ok := entry.Transport.(contract.SerialConn)
	if !ok {
		s.fail(op, w, wire.E("CAPABILITY_UNSUPPORTED", "no serial transact"))
		return s.final(op.ID, requestID), nil
	}
	m := contract.Matcher{Literal: wait}
	if strings.HasPrefix(wait, "re:") {
		re, rerr := regexp.Compile(strings.TrimPrefix(wait, "re:"))
		if rerr != nil {
			s.fail(op, w, wire.E("INVALID_ARGUMENT", rerr.Error()))
			return s.final(op.ID, requestID), nil
		}
		m = contract.Matcher{Regex: re}
	}
	body := append([]byte(payload), '\n')
	got, err := sc.Transact(jctx, body, m, s.Limits.MatcherMaxBytes)
	_, _ = w.Append("data", string(got), nil)
	if err != nil {
		s.fail(op, w, err)
		return s.final(op.ID, requestID), nil
	}
	_, _ = w.Append("completed", "", map[string]any{"state": "succeeded"})
	zero := 0
	_ = s.DB.Finish(ctx, op.ID, op.Version, "succeeded", "", &zero, w.Cursor(), false)
	r := s.final(op.ID, requestID)
	r["data"] = string(got)
	return r, nil
}

func (s *Service) Wait(ctx context.Context, requestID, jobID string) (wire.Result, error) {
	op, err := s.DB.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	for !storage.Terminal(op.State) {
		select {
		case <-ctx.Done():
			return s.snapshot(jobID, requestID, true), nil
		case <-time.After(80 * time.Millisecond):
			op, err = s.DB.Get(ctx, jobID)
			if err != nil {
				return nil, err
			}
		}
	}
	return s.final(jobID, requestID), nil
}

func (s *Service) Follow(ctx context.Context, requestID, jobID string, after int64, emit func(wire.Event)) (wire.Result, error) {
	op, err := s.DB.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	evs, err := logstore.Replay(op.EventPath, after)
	if err != nil {
		return nil, err
	}
	for _, ev := range evs {
		if emit != nil {
			emit(ev)
		}
		if ev.Cursor > after {
			after = ev.Cursor
		}
	}
	if storage.Terminal(op.State) {
		return s.final(jobID, requestID), nil
	}
	s.mu.Lock()
	live := s.live[jobID]
	s.mu.Unlock()
	if live == nil {
		for !storage.Terminal(op.State) {
			select {
			case <-ctx.Done():
				return s.snapshot(jobID, requestID, true), nil
			case <-time.After(100 * time.Millisecond):
				op, _ = s.DB.Get(ctx, jobID)
				evs, _ = logstore.Replay(op.EventPath, after)
				for _, ev := range evs {
					emit(ev)
					after = ev.Cursor
				}
			}
		}
		return s.final(jobID, requestID), nil
	}
	<-live.done
	op, _ = s.DB.Get(ctx, jobID)
	evs, _ = logstore.Replay(op.EventPath, after)
	for _, ev := range evs {
		emit(ev)
	}
	return s.final(jobID, requestID), nil
}

func (s *Service) Cancel(ctx context.Context, requestID, jobID string) (wire.Result, error) {
	op, err := s.DB.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if storage.Terminal(op.State) {
		return s.final(jobID, requestID), nil
	}
	s.mu.Lock()
	if r := s.live[jobID]; r != nil {
		r.cancel()
	}
	s.mu.Unlock()
	return s.Wait(ctx, requestID, jobID)
}

func (s *Service) List(ctx context.Context, requestID string) (wire.Result, error) {
	ops, err := s.DB.ListJobs(ctx, 500)
	if err != nil {
		return nil, err
	}
	jobs := make([]map[string]any, 0, len(ops))
	for _, op := range ops {
		item := map[string]any{"id": op.ID, "target": op.TargetRef, "kind": op.Kind, "status": op.State, "started_at": rfc(op.StartedAt)}
		if op.FinishedAt != nil {
			item["finished_at"] = rfc(op.FinishedAt)
		}
		jobs = append(jobs, item)
	}
	r := wire.Base(true, requestID, "ok")
	r["jobs"] = jobs
	if len(ops) >= 500 {
		r["truncated"] = true
	}
	return r, nil
}

func (s *Service) Recover(ctx context.Context) error {
	ops, err := s.DB.Incomplete(ctx)
	if err != nil {
		return err
	}
	for _, op := range ops {
		state := "execution_unknown"
		if op.Kind == "cp" {
			state = "failed"
		}
		if op.State == "queued" && op.DispatchedAt == nil {
			state = "execution_unknown"
		}
		_ = s.DB.Finish(ctx, op.ID, op.Version, state, "EXECUTION_UNKNOWN", nil, op.NextCursor, false)
	}
	return nil
}

func (s *Service) createOp(ctx context.Context, requestID, kind string, t *config.Resolved, reqBody map[string]any) (*storage.Operation, *logstore.Writer, error) {
	if existing, err := s.DB.GetByRequest(ctx, requestID); err == nil && existing != nil {
		w, _ := s.Logs.Open(filepath.Dir(existing.EventPath), existing.ID, existing.NextCursor)
		return existing, w, nil
	}
	id := wire.NewJobID()
	dir := filepath.Join(s.DataDir, "logs", "operations", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	body, _ := json.Marshal(reqBody)
	_ = os.WriteFile(filepath.Join(dir, "request.json"), body, 0o600)
	w, err := s.Logs.Open(dir, id, 0)
	if err != nil {
		return nil, nil, err
	}
	op := &storage.Operation{
		ID:           id,
		RequestID:    requestID,
		Kind:         kind,
		TargetRef:    t.Ref,
		ResolvedJSON: storage.MustJSON(t),
		State:        "queued",
		EventPath:    w.Path(),
		StdoutPath:   filepath.Join(dir, "stdout.log"),
		StderrPath:   filepath.Join(dir, "stderr.log"),
		Version:      1,
		CreatedAt:    storage.NowUS(),
	}
	if err := s.DB.InsertOperation(ctx, op); err != nil {
		if wire.Is(err, "INVALID_ARGUMENT") {
			if existing, gerr := s.DB.GetByRequest(ctx, requestID); gerr == nil && existing != nil {
				w, _ := s.Logs.Open(filepath.Dir(existing.EventPath), existing.ID, existing.NextCursor)
				return existing, w, nil
			}
		}
		return nil, nil, wire.E("STORAGE_ERROR", err.Error())
	}
	return op, w, nil
}

func (s *Service) fail(op *storage.Operation, w *logstore.Writer, err error) {
	e, ok := err.(*wire.Error)
	if !ok {
		e = wire.E("EXECUTION_UNKNOWN", err.Error())
	}
	_, _ = w.Append("completed", "", map[string]any{"state": e.Status, "error_code": e.Code, "message": e.Message})
	_ = s.DB.Finish(context.Background(), op.ID, op.Version, e.Status, e.Code, nil, w.Cursor(), false)
}

func (s *Service) snapshot(id, requestID string, ok bool) wire.Result {
	op, err := s.DB.Get(context.Background(), id)
	if err != nil {
		return wire.Fail(requestID, err)
	}
	r := wire.Base(ok, requestID, op.State)
	r["operation_id"] = op.ID
	r["target"] = op.TargetRef
	r["log_path"] = filepath.Dir(op.EventPath)
	return r
}

func (s *Service) final(id, requestID string) wire.Result {
	op, err := s.DB.Get(context.Background(), id)
	if err != nil {
		return wire.Fail(requestID, err)
	}
	ok := op.State == "succeeded"
	r := wire.Base(ok, requestID, op.State)
	r["operation_id"] = op.ID
	r["target"] = op.TargetRef
	r["log_path"] = filepath.Dir(op.EventPath)
	if op.ErrorCode != "" {
		r["error_code"] = op.ErrorCode
		r["retryable"] = wire.E(op.ErrorCode, "").Retryable
		r["message"] = op.ErrorCode
	}
	if op.ExitCode != nil {
		r["exit_code"] = *op.ExitCode
	} else if op.State == "execution_unknown" {
		r["exit_code"] = nil
	}
	evs, _ := logstore.Replay(op.EventPath, 0)
	if op.ErrorCode != "" {
		if msg := completedMessage(evs); msg != "" {
			r["message"] = msg
		}
	}
	stdout, stderr := logstore.CollectStd(evs)
	max := s.Limits.MaxResponseBytes
	var trunc bool
	stdout, t1 := wire.TruncateUTF8(stdout, max)
	stderr, t2 := wire.TruncateUTF8(stderr, max)
	trunc = t1 || t2 || op.OutputTruncated
	if stdout != "" || op.Kind == "run" {
		r["stdout"] = stdout
		r["stderr"] = stderr
	}
	if trunc {
		r["truncated"] = true
	}
	if op.Kind == "deploy" {
		if dep, err := s.DB.GetDeployment(context.Background(), op.ID); err == nil {
			r["kind"] = "deploy"
			r["current_step"] = dep.CurrentStep
			r["deploy_status"] = dep.DeployStatus
			r["artifact_sha256"] = dep.ArtifactSHA256
		}
	}
	if op.Kind == "serial_exec" {
		var data strings.Builder
		for _, ev := range evs {
			if ev.Type == "data" {
				data.WriteString(ev.Data)
			}
		}
		r["data"] = data.String()
	}
	return r
}

func completedMessage(evs []wire.Event) string {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == "completed" && evs[i].Message != "" {
			return evs[i].Message
		}
	}
	return ""
}

func (s *Service) track(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	s.live[id] = &running{cancel: cancel, done: make(chan struct{})}
	s.mu.Unlock()
}

func (s *Service) untrack(id string) {
	s.mu.Lock()
	if r := s.live[id]; r != nil {
		close(r.done)
		delete(s.live, id)
	}
	s.mu.Unlock()
}

type chunkWriter struct {
	w   *logstore.Writer
	typ string
}

func (c *chunkWriter) Write(p []byte) (int, error) {
	_, err := c.w.Append(c.typ, string(p), nil)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func hide(cmd string, sensitive bool) any {
	if sensitive {
		return map[string]any{"length": len(cmd)}
	}
	return cmd
}

func rfc(us *int64) string {
	if us == nil {
		return ""
	}
	return time.UnixMicro(*us).UTC().Format(time.RFC3339Nano)
}

type copySpec struct {
	Direction string
	Target    string
	Local     string
	Remote    string
}

func parseCopy(src, dst string) (*copySpec, error) {
	sr, sRemote := splitRemote(src)
	dr, dRemote := splitRemote(dst)
	switch {
	case sr != "" && dr == "":
		return &copySpec{Direction: "download", Target: sr, Remote: sRemote, Local: dst}, nil
	case sr == "" && dr != "":
		return &copySpec{Direction: "upload", Target: dr, Remote: dRemote, Local: src}, nil
	case sr != "" && dr != "":
		return nil, wire.E("INVALID_ARGUMENT", "two remote paths are not supported")
	default:
		return nil, wire.E("INVALID_ARGUMENT", "one path must be target:remote")
	}
}

func splitRemote(p string) (target, remote string) {
	if len(p) >= 3 {
		c0, c1, c2 := p[0], p[1], p[2]
		if c1 == ':' && ((c0 >= 'A' && c0 <= 'Z') || (c0 >= 'a' && c0 <= 'z')) && c2 == '\\' {
			return "", p
		}
	}
	i := strings.Index(p, ":")
	if i <= 0 {
		return "", p
	}
	return p[:i], p[i+1:]
}

func underRoot(remote, root string) bool {
	return workspace.Under(remote, root)
}

func shaFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
