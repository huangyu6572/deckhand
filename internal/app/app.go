package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"localaihub/internal/config"
	"localaihub/internal/job"
	"localaihub/internal/log"
	"localaihub/internal/platform"
	"localaihub/internal/secrets"
	"localaihub/internal/session"
	"localaihub/internal/storage"
	"localaihub/internal/transport/contract"
	"localaihub/internal/transport/pool"
	"localaihub/internal/transport/registry"
	serialx "localaihub/internal/transport/serial"
	sshx "localaihub/internal/transport/ssh"
	"localaihub/internal/wire"
)

type App struct {
	DataDir  string
	Settings config.Settings
	File     config.File
	DB       *storage.DB
	Logs     *logstore.Store
	Pool     *pool.Pool
	Jobs     *job.Service
	Sess     *session.Service
	Reg      *registry.Registry
	mu       *platform.Mutex
	ln       net.Listener
}

func Open() (*App, error) {
	dir, err := platform.DataDir()
	if err != nil {
		return nil, err
	}
	return OpenDir(dir)
}

func OpenDir(dir string) (*App, error) {
	for _, sub := range []string{"db", "logs", "runtime", "ssh", "recipes"} {
		if err := platform.EnsureDir(filepath.Join(dir, sub)); err != nil {
			return nil, err
		}
	}
	settings, err := config.LoadSettings(dir)
	if err != nil {
		return nil, err
	}
	file, err := config.LoadConnections(dir)
	if err != nil {
		return nil, err
	}
	db, err := storage.Open(dir)
	if err != nil {
		return nil, err
	}
	logs := logstore.New(settings.Limits.MaxEventBytes, settings.Limits.MaxOperationLogBytes)
	reg := registry.New()
	reg.Register("ssh", sshx.Factory)
	reg.Register("serial", serialx.Factory)
	opts := contract.Options{
		DataDir: dir,
		OnHostKeyNew: func(host, comment string) {
			os.Stderr.WriteString("host key recorded for " + host + " (" + comment + ")\n")
		},
		Warn: func(m string) { os.Stderr.WriteString(m + "\n") },
	}
	p := pool.New(reg, opts, settings.Connection.IdleTimeout.Duration())
	home, _ := os.UserHomeDir()
	sshCfg := filepath.Join(home, ".ssh", "config")
	resolve := func(ctx context.Context, ref string, allowPublic bool) (*config.Resolved, error) {
		r := &config.Resolver{DataDir: dir, Settings: settings, File: file, SSHConfig: sshCfg, AllowPublic: allowPublic}
		return r.Resolve(ctx, ref)
	}
	jobs := job.New(dir, db, logs, p, settings.Limits, resolve)
	sess := session.New(dir, db, logs, p, resolve)
	if err := jobs.Recover(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	sess.RecoverSSH()
	a := &App{DataDir: dir, Settings: settings, File: file, DB: db, Logs: logs, Pool: p, Jobs: jobs, Sess: sess, Reg: reg}
	return a, nil
}

func (a *App) Serve() error {
	sid, err := platform.CurrentSID()
	if err != nil {
		return err
	}
	mu, exists, err := platform.TryCreateInstanceMutex(platform.InstanceMutexName(sid))
	if err != nil {
		return err
	}
	if exists {
		return wire.E("DAEMON_INSTANCE_CONFLICT", "hubd already running")
	}
	a.mu = mu
	lock := filepath.Join(a.DataDir, "runtime", "hubd.lock")
	exe, _ := os.Executable()
	body := wire.MustJSON(map[string]any{
		"pid": os.Getpid(), "started_at": time.Now().UTC().Format(time.RFC3339Nano),
		"exe": exe, "protocol": wire.ProtocolVersion,
	})
	_ = platform.AtomicWriteFile(lock, body, 0o600)
	ln, err := platform.ListenPipe(sid)
	if err != nil {
		return err
	}
	a.ln = ln
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go a.handleConn(c)
	}
}

func (a *App) Close() {
	if a.ln != nil {
		_ = a.ln.Close()
	}
	a.Pool.CloseAll()
	_ = a.DB.Close()
	if a.mu != nil {
		a.mu.Close()
	}
}

func (a *App) handleConn(c net.Conn) {
	defer c.Close()
	max := a.Settings.Limits.PipeMaxFrameBytes
	fr, err := wire.ReadFrame(c, max)
	if err != nil {
		ok := false
		_ = wire.WriteFrame(c, &wire.Frame{Kind: wire.KindResponse, ID: "", OK: &ok, Payload: wire.MustJSON(wire.Fail("req_invalid", err))})
		return
	}
	if fr.Kind != wire.KindRequest {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.dispatch(ctx, cancel, c, fr)
}

func (a *App) writeResult(c io.Writer, id string, r wire.Result, err error) {
	if err != nil {
		r = wire.Fail(id, err)
	}
	ok := false
	if v, exists := r["ok"].(bool); exists {
		ok = v
	}
	_ = wire.WriteFrame(c, &wire.Frame{Kind: wire.KindResponse, ID: id, OK: &ok, Payload: wire.MustJSON(r)})
}

func (a *App) writeRunning(c io.Writer, id string, r wire.Result) {
	r["stream"] = true
	if r["status"] == nil {
		r["status"] = "running"
	}
	ok := true
	_ = wire.WriteFrame(c, &wire.Frame{Kind: wire.KindResponse, ID: id, OK: &ok, Payload: wire.MustJSON(r)})
}

func (a *App) writeEvent(c io.Writer, id string, ev wire.Event) {
	_ = wire.WriteFrame(c, &wire.Frame{Kind: wire.KindEvent, ID: id, Event: wire.MustJSON(ev)})
}

func (a *App) dispatch(ctx context.Context, cancel context.CancelFunc, c net.Conn, fr *wire.Frame) {
	var params map[string]any
	_ = json.Unmarshal(fr.Params, &params)
	if params == nil {
		params = map[string]any{}
	}
	str := func(k string) string {
		v, _ := params[k].(string)
		return v
	}
	boolf := func(k string) bool {
		v, _ := params[k].(bool)
		return v
	}
	int64f := func(k string) int64 {
		switch v := params[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case json.Number:
			n, _ := v.Int64()
			return n
		}
		return 0
	}
	timeout := time.Duration(int64f("timeout_ms")) * time.Millisecond
	allow := boolf("allow_public")
	reqID := fr.ID

	switch fr.Method {
	case wire.JobRun:
		req := job.RunReq{
			Target: str("target"), Command: str("command"), Timeout: timeout,
			AllowPublic: allow, JSONL: boolf("jsonl"), Detach: boolf("detach"),
			Sensitive: boolf("sensitive"), RequestID: reqID,
		}
		var emit func(wire.Event)
		if req.JSONL && !req.Detach {
			a.writeRunning(c, reqID, wire.Base(true, reqID, "running"))
			emit = func(ev wire.Event) { a.writeEvent(c, reqID, ev) }
		}
		r, err := a.Jobs.Run(ctx, req, emit, req.JSONL)
		a.writeResult(c, reqID, r, err)
	case wire.JobWait:
		r, err := a.Jobs.Wait(ctx, reqID, str("job_id"))
		a.writeResult(c, reqID, r, err)
	case wire.JobFollow:
		a.writeRunning(c, reqID, wire.Base(true, reqID, "running"))
		r, err := a.Jobs.Follow(ctx, reqID, str("job_id"), int64f("after_cursor"), func(ev wire.Event) {
			a.writeEvent(c, reqID, ev)
		})
		a.writeResult(c, reqID, r, err)
	case wire.JobCancel:
		r, err := a.Jobs.Cancel(ctx, reqID, str("job_id"))
		a.writeResult(c, reqID, r, err)
	case wire.JobList:
		r, err := a.Jobs.List(ctx, reqID)
		a.writeResult(c, reqID, r, err)
	case wire.FileCopy:
		r, err := a.Jobs.Copy(ctx, reqID, str("src"), str("dst"), timeout, allow)
		a.writeResult(c, reqID, r, err)
	case wire.SerialExec:
		r, err := a.Jobs.SerialExec(ctx, reqID, str("target"), str("payload"), str("wait"), timeout, int(int64f("baud")), allow)
		a.writeResult(c, reqID, r, err)
	case wire.SerialList:
		ports, err := serialx.List()
		if err != nil {
			a.writeResult(c, reqID, nil, err)
			return
		}
		r := wire.Base(true, reqID, "ok")
		r["ports"] = ports
		a.writeResult(c, reqID, r, nil)
	case wire.DeployStart:
		r, err := a.Jobs.Deploy(ctx, reqID, str("target"), str("recipe"), str("artifact"), str("idempotency_key"), timeout, allow)
		a.writeResult(c, reqID, r, err)
	case wire.TargetList:
		names := make([]map[string]any, 0)
		for n, t := range a.File.Targets {
			names = append(names, map[string]any{"name": n, "transport": t.Transport, "ephemeral": false})
		}
		r := wire.Base(true, reqID, "ok")
		r["targets"] = names
		a.writeResult(c, reqID, r, nil)
	case wire.SessionOpen:
		r, err := a.Sess.Open(ctx, reqID, str("target"), str("name"), allow)
		a.writeResult(c, reqID, r, err)
	case wire.SessionExec:
		r, err := a.Sess.Exec(ctx, reqID, str("session_id"), str("command"), timeout, boolf("no_sentinel"))
		a.writeResult(c, reqID, r, err)
	case wire.SessionRead:
		wait := time.Duration(int64f("wait_ms")) * time.Millisecond
		r, err := a.Sess.Read(ctx, reqID, str("session_id"), int64f("after_cursor"), wait, boolf("bytes_b64"), boolf("jsonl"))
		a.writeResult(c, reqID, r, err)
	case wire.SessionWrite:
		r, err := a.Sess.Write(ctx, reqID, str("session_id"), str("data"), str("data_base64"))
		a.writeResult(c, reqID, r, err)
	case wire.SessionResize:
		r, err := a.Sess.Resize(ctx, reqID, str("session_id"), int(int64f("cols")), int(int64f("rows")))
		a.writeResult(c, reqID, r, err)
	case wire.SessionClose:
		r, err := a.Sess.Close(ctx, reqID, str("session_id"))
		a.writeResult(c, reqID, r, err)
	case wire.SessionAttach:
		a.writeRunning(c, reqID, wire.Base(true, reqID, "open"))
		go a.holdStream(c, cancel, func(in *wire.Frame) {
			a.applyStreamInput(ctx, in)
		})
		_ = a.Sess.Attach(ctx, str("session_id"), func(ev wire.Event) { a.writeEvent(c, reqID, ev) })
	case wire.SessionDetach:
		r, err := a.Sess.Detach(ctx, reqID, str("session_id"))
		a.writeResult(c, reqID, r, err)
	case wire.ConnectionList:
		list := a.Pool.List()
		arr := make([]map[string]any, 0, len(list))
		for _, e := range list {
			arr = append(arr, map[string]any{
				"id": e.ID, "target_ref": e.Target.Ref, "transport": e.Target.Transport,
				"state": e.State, "idle_seconds": pool.IdleSeconds(e),
			})
		}
		r := wire.Base(true, reqID, "ok")
		r["connections"] = arr
		a.writeResult(c, reqID, r, nil)
	case wire.ConnectionStatus:
		e := a.Pool.Find(str("id"))
		if e == nil {
			a.writeResult(c, reqID, nil, wire.E("CONNECTION_NOT_FOUND", "unknown connection"))
			return
		}
		r := wire.Base(true, reqID, e.State)
		r["id"] = e.ID
		r["target_ref"] = e.Target.Ref
		r["transport"] = e.Target.Transport
		r["idle_seconds"] = pool.IdleSeconds(e)
		a.writeResult(c, reqID, r, nil)
	case wire.ConnectionClose:
		id := str("id")
		err := a.Pool.Close(id)
		if err != nil && !wire.Is(err, "CONNECTION_NOT_FOUND") {
			a.writeResult(c, reqID, nil, err)
			return
		}
		r := wire.Base(true, reqID, "closed")
		a.writeResult(c, reqID, r, nil)
	case wire.SecretSet:
		err := secrets.Set(str("target"), str("secret"))
		r := wire.Base(err == nil, reqID, "ok")
		r["target"] = str("target")
		a.writeResult(c, reqID, r, err)
	case wire.SecretDelete:
		err := secrets.Delete(str("target"))
		a.writeResult(c, reqID, wire.Base(true, reqID, "ok"), err)
	case wire.SerialMonitor:
		a.writeRunning(c, reqID, wire.Base(true, reqID, "open"))
		go a.holdStream(c, cancel, nil)
		err := a.Sess.Monitor(ctx, reqID, str("target"), int64f("after_cursor"), allow, func(ev wire.Event) {
			a.writeEvent(c, reqID, ev)
		})
		if err != nil {
			a.writeResult(c, reqID, nil, err)
		}
	default:
		a.writeResult(c, reqID, nil, wire.E("INVALID_ARGUMENT", "unknown method "+fr.Method))
	}
}

func (a *App) holdStream(c net.Conn, cancel context.CancelFunc, onFrame func(*wire.Frame)) {
	max := a.Settings.Limits.PipeMaxFrameBytes
	for {
		in, err := wire.ReadFrame(c, max)
		if err != nil {
			cancel()
			return
		}
		if onFrame != nil {
			onFrame(in)
		}
	}
}

func (a *App) applyStreamInput(ctx context.Context, fr *wire.Frame) {
	if fr == nil || fr.Kind != wire.KindRequest {
		return
	}
	var params map[string]any
	_ = json.Unmarshal(fr.Params, &params)
	str := func(k string) string {
		if params == nil {
			return ""
		}
		v, _ := params[k].(string)
		return v
	}
	int64f := func(k string) int64 {
		if params == nil {
			return 0
		}
		switch v := params[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		}
		return 0
	}
	switch fr.Method {
	case wire.SessionWrite:
		_, _ = a.Sess.Write(ctx, fr.ID, str("session_id"), str("data"), str("data_base64"))
	case wire.SessionResize:
		_, _ = a.Sess.Resize(ctx, fr.ID, str("session_id"), int(int64f("cols")), int(int64f("rows")))
	case wire.SessionDetach:
		_, _ = a.Sess.Detach(ctx, fr.ID, str("session_id"))
	}
}
