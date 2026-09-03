package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"localaihub/internal/config"
	"localaihub/internal/log"
	"localaihub/internal/storage"
	"localaihub/internal/transport/contract"
	"localaihub/internal/transport/pool"
	"localaihub/internal/wire"
)

type liveSess struct {
	id     string
	name   string
	kind   string
	state  string
	row    *storage.Session
	target *config.Resolved
	pty    contract.PTY
	serial contract.SerialConn
	entry  *pool.Entry
	w      *logstore.Writer
	mu     sync.Mutex
	busy   bool
}

type Service struct {
	DataDir string
	DB      *storage.DB
	Logs    *logstore.Store
	Pool    *pool.Pool
	Resolve func(ctx context.Context, ref string, allowPublic bool) (*config.Resolved, error)
	mu      sync.Mutex
	live    map[string]*liveSess
	byName  map[string]*liveSess
	attach  map[string][]context.CancelFunc
}

func New(dataDir string, db *storage.DB, logs *logstore.Store, p *pool.Pool, resolve func(context.Context, string, bool) (*config.Resolved, error)) *Service {
	return &Service{
		DataDir: dataDir, DB: db, Logs: logs, Pool: p, Resolve: resolve,
		live: map[string]*liveSess{}, byName: map[string]*liveSess{}, attach: map[string][]context.CancelFunc{},
	}
}

func (s *Service) Open(ctx context.Context, requestID, target, name string, allowPublic bool) (wire.Result, error) {
	t, err := s.Resolve(ctx, target, allowPublic)
	if err != nil {
		return nil, err
	}
	if t.Transport != "ssh" {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "session requires SSH PTY")
	}
	if name != "" {
		if r, err := s.existingNamed(ctx, requestID, name); r != nil || err != nil {
			return r, err
		}
	}
	now := storage.NowUS()
	id := wire.NewSessID()
	if name == "" {
		name = id
	}
	dir := filepath.Join(s.DataDir, "logs", "sessions", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	row := &storage.Session{
		ID: id, Name: name, TargetRef: t.Ref, Kind: "ssh_pty", State: "opening",
		EventPath: eventPath, CreatedAt: now, LastActivityAt: now, Version: 1,
	}
	if err := s.DB.InsertSession(ctx, row); err != nil {
		if wire.Is(err, "INVALID_ARGUMENT") {
			if r, e2 := s.existingNamed(ctx, requestID, name); r != nil || e2 != nil {
				return r, e2
			}
		}
		return nil, err
	}
	entry, err := s.Pool.Acquire(ctx, t)
	if err != nil {
		s.failRow(ctx, row, "failed")
		return nil, err
	}
	pc, ok := entry.Transport.(contract.PTYConn)
	if !ok {
		s.Pool.Release(entry)
		s.failRow(ctx, row, "failed")
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "no pty")
	}
	pty, err := pc.OpenPTY(ctx, 80, 24)
	if err != nil {
		s.Pool.Release(entry)
		s.failRow(ctx, row, "failed")
		return nil, err
	}
	w, err := s.Logs.Open(dir, id, 0)
	if err != nil {
		pty.Close()
		s.Pool.Release(entry)
		s.failRow(ctx, row, "failed")
		return nil, err
	}
	row.State = "open"
	row.EventPath = w.Path()
	if err := s.DB.UpdateSession(ctx, row); err != nil {
		pty.Close()
		s.Pool.Release(entry)
		return nil, err
	}
	ls := &liveSess{id: id, name: name, kind: "ssh_pty", state: "open", row: row, target: t, pty: pty, entry: entry, w: w}
	s.putLive(ls)
	go s.pumpPTY(ls)
	return s.openResult(requestID, ls), nil
}

func (s *Service) existingNamed(ctx context.Context, requestID, name string) (wire.Result, error) {
	s.mu.Lock()
	if ex := s.byName[name]; ex != nil {
		s.mu.Unlock()
		return s.openResult(requestID, ex), nil
	}
	s.mu.Unlock()
	row, err := s.DB.GetActiveSessionByName(ctx, name)
	if err != nil || row == nil {
		return nil, err
	}
	s.mu.Lock()
	if ex := s.live[row.ID]; ex != nil {
		s.mu.Unlock()
		return s.openResult(requestID, ex), nil
	}
	s.mu.Unlock()
	r := wire.Base(true, requestID, row.State)
	r["operation_id"] = row.ID
	r["kind"] = row.Kind
	r["target"] = row.TargetRef
	return r, nil
}

func (s *Service) openResult(requestID string, ls *liveSess) wire.Result {
	r := wire.Base(true, requestID, ls.state)
	r["operation_id"] = ls.id
	r["kind"] = ls.kind
	if ls.target != nil {
		r["target"] = ls.target.Ref
	}
	return r
}

func (s *Service) putLive(ls *liveSess) {
	s.mu.Lock()
	s.live[ls.id] = ls
	s.byName[ls.name] = ls
	s.mu.Unlock()
}

func (s *Service) dropLive(ls *liveSess) {
	s.mu.Lock()
	delete(s.live, ls.id)
	if s.byName[ls.name] == ls {
		delete(s.byName, ls.name)
	}
	s.mu.Unlock()
}

func (s *Service) failRow(ctx context.Context, row *storage.Session, state string) {
	row.State = state
	_ = s.DB.SetSessionState(ctx, row.ID, state, row.NextCursor)
}

func (s *Service) persist(ls *liveSess, state string) {
	ls.mu.Lock()
	ls.state = state
	row := ls.row
	cur := int64(0)
	if ls.w != nil {
		cur = ls.w.Cursor()
	}
	ls.mu.Unlock()
	if row == nil {
		return
	}
	row.State = state
	row.NextCursor = cur
	_ = s.DB.SetSessionState(context.Background(), row.ID, state, cur)
	row.Version++
}

func (s *Service) pumpPTY(ls *liveSess) {
	buf := make([]byte, 4096)
	for {
		n, err := ls.pty.Read(buf)
		if n > 0 {
			_, _ = ls.w.Append("stdout", string(buf[:n]), nil)
		}
		if err != nil {
			s.persist(ls, "disconnected")
			return
		}
	}
}

func (s *Service) pumpSerial(ls *liveSess) {
	if ls.serial == nil {
		return
	}
	for b := range ls.serial.ReadLoop() {
		_, _ = ls.w.Append("data", string(b), nil)
	}
	s.persist(ls, "disconnected")
}

func (s *Service) getLive(id string) (*liveSess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ls := s.live[id]; ls != nil {
		return ls, nil
	}
	if ls := s.byName[id]; ls != nil {
		return ls, nil
	}
	return nil, wire.E("SESSION_NOT_FOUND", "unknown session")
}

func (s *Service) resolveRow(ctx context.Context, id string) (*storage.Session, error) {
	row, err := s.DB.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if row != nil {
		return row, nil
	}
	row, err = s.DB.GetActiveSessionByName(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, wire.E("SESSION_NOT_FOUND", "unknown session")
	}
	return row, nil
}

func (s *Service) Exec(ctx context.Context, requestID, sess, command string, timeout time.Duration, noSentinel bool) (wire.Result, error) {
	ls, err := s.getLive(sess)
	if err != nil {
		return nil, err
	}
	if ls.pty == nil {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "session exec requires SSH PTY")
	}
	ls.mu.Lock()
	if ls.busy {
		ls.mu.Unlock()
		return nil, wire.E("SESSION_BUSY", "session is busy")
	}
	ls.busy = true
	ls.mu.Unlock()
	defer func() { ls.mu.Lock(); ls.busy = false; ls.mu.Unlock() }()
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	nonce := randNonce()
	cmd := command
	if !noSentinel {
		cmd = fmt.Sprintf("{ %s\nprintf '\\n__HUB_DONE_%s_%%s__\\n' \"$?\"; }", command, nonce)
	}
	mark := ls.w.Cursor()
	if _, err := ls.pty.Write([]byte(cmd + "\n")); err != nil {
		return nil, wire.E("REMOTE_UNREACHABLE", err.Error())
	}
	if noSentinel {
		r := wire.Base(true, requestID, "open")
		r["operation_id"] = ls.id
		return r, nil
	}
	needle := "__HUB_DONE_" + nonce + "_"
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, wire.E("JOB_TIMEOUT", "session exec timed out")
		case <-time.After(40 * time.Millisecond):
		}
		evs, _ := logstore.Replay(ls.w.Path(), mark)
		var b strings.Builder
		for _, ev := range evs {
			if ev.Type == "stdout" {
				b.WriteString(logstore.EventBytes(ev))
			}
		}
		text := b.String()
		if i := strings.Index(text, needle); i >= 0 {
			rest := text[i+len(needle):]
			end := strings.Index(rest, "__")
			code := 0
			if end > 0 {
				fmt.Sscanf(rest[:end], "%d", &code)
			}
			out := text[:i]
			ok := code == 0
			r := wire.Base(ok, requestID, mapStatus(ok))
			r["operation_id"] = ls.id
			r["exit_code"] = code
			r["stdout"] = out
			if !ok {
				r["error_code"] = "REMOTE_EXIT_NONZERO"
				r["retryable"] = false
				r["message"] = "remote exit nonzero"
			}
			return r, nil
		}
	}
	return nil, wire.E("JOB_TIMEOUT", "session exec timed out")
}

func mapStatus(ok bool) string {
	if ok {
		return "open"
	}
	return "failed"
}

func (s *Service) Write(ctx context.Context, requestID, sess, data, dataB64 string) (wire.Result, error) {
	_ = ctx
	raw := []byte(data)
	if dataB64 != "" {
		b, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return nil, wire.E("INVALID_ARGUMENT", "invalid --bytes-b64")
		}
		raw = b
	}
	ls, err := s.getLive(sess)
	if err != nil {
		return nil, err
	}
	var n int
	switch {
	case ls.pty != nil:
		n, err = ls.pty.Write(raw)
	case ls.serial != nil:
		n, err = writeSerial(ls.serial, raw)
	default:
		return nil, wire.E("REMOTE_UNREACHABLE", "session has no stream")
	}
	if err != nil {
		return nil, wire.E("REMOTE_UNREACHABLE", err.Error())
	}
	r := wire.Base(true, requestID, "open")
	r["operation_id"] = ls.id
	r["bytes_written"] = n
	return r, nil
}

func writeSerial(sc contract.SerialConn, raw []byte) (int, error) {
	if w, ok := sc.(io.Writer); ok {
		return w.Write(raw)
	}
	got, err := sc.Transact(context.Background(), raw, contract.Matcher{}, 1)
	_ = got
	if err != nil && !wire.Is(err, "SERIAL_TIMEOUT") {
		return 0, err
	}
	return len(raw), nil
}

func (s *Service) Resize(ctx context.Context, requestID, sess string, cols, rows int) (wire.Result, error) {
	_ = ctx
	if cols <= 0 || rows <= 0 {
		return nil, wire.E("INVALID_ARGUMENT", "cols and rows must be positive")
	}
	ls, err := s.getLive(sess)
	if err != nil {
		return nil, err
	}
	if ls.pty == nil {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "resize requires SSH PTY")
	}
	if err := ls.pty.Resize(cols, rows); err != nil {
		return nil, err
	}
	r := wire.Base(true, requestID, "open")
	r["operation_id"] = ls.id
	return r, nil
}

func (s *Service) Read(ctx context.Context, requestID, sess string, after int64, wait time.Duration, bytesB64, wantEvents bool) (wire.Result, error) {
	path, opID, status, eof := "", "", "open", false
	var w *logstore.Writer
	if ls, err := s.getLive(sess); err == nil {
		path = ls.w.Path()
		opID = ls.id
		status = ls.state
		w = ls.w
		eof = ls.state == "closed" || ls.state == "failed"
	} else {
		row, rerr := s.resolveRow(ctx, sess)
		if rerr != nil {
			return nil, rerr
		}
		path = row.EventPath
		opID = row.ID
		status = row.State
		eof = row.State == "closed" || row.State == "failed"
	}
	collect := func() ([]wire.Event, error) {
		return logstore.Replay(path, after)
	}
	evs, err := collect()
	if err != nil {
		return nil, err
	}
	if wait > 0 && len(evs) == 0 && !eof {
		deadline := time.Now().Add(wait)
		if w != nil {
			ch, unsub := w.Subscribe(8)
			defer unsub()
			for time.Now().Before(deadline) && len(evs) == 0 {
				select {
				case <-ctx.Done():
					evs, _ = collect()
					break
				case ev, ok := <-ch:
					if !ok {
						evs, _ = collect()
						break
					}
					if ev.Cursor > after {
						evs = append(evs, ev)
					}
				case <-time.After(40 * time.Millisecond):
					evs, _ = collect()
				}
			}
		} else {
			for time.Now().Before(deadline) && len(evs) == 0 {
				select {
				case <-ctx.Done():
					break
				case <-time.After(40 * time.Millisecond):
					evs, _ = collect()
				}
			}
		}
	}
	first, _, _ := logstore.Bounds(path)
	lost := int64(0)
	if after > 0 {
		if first == 0 {
			lost = after
		} else if first > after+1 {
			lost = first - after - 1
		}
	} else if first > 1 {
		lost = first - 1
	}
	var data strings.Builder
	next := after
	for _, ev := range evs {
		if ev.Type == "stdout" || ev.Type == "data" {
			data.WriteString(logstore.EventBytes(ev))
		}
		if ev.Cursor > next {
			next = ev.Cursor
		}
	}
	r := wire.Base(true, requestID, status)
	r["operation_id"] = opID
	if bytesB64 {
		r["data_base64"] = base64.StdEncoding.EncodeToString([]byte(data.String()))
	} else {
		r["data"] = data.String()
	}
	r["next_cursor"] = next
	r["events_lost"] = lost
	r["first_available_cursor"] = first
	r["eof"] = eof
	if wantEvents {
		r["events"] = evs
	}
	return r, nil
}

func (s *Service) Close(ctx context.Context, requestID, sess string) (wire.Result, error) {
	if ls, err := s.getLive(sess); err == nil {
		s.cancelAttach(ls.id)
		if ls.pty != nil {
			_ = ls.pty.Close()
		}
		if ls.entry != nil {
			s.Pool.Release(ls.entry)
		}
		s.dropLive(ls)
		s.persist(ls, "closed")
		r := wire.Base(true, requestID, "closed")
		r["operation_id"] = ls.id
		return r, nil
	}
	row, err := s.resolveRow(ctx, sess)
	if err != nil {
		r := wire.Base(true, requestID, "closed")
		return r, nil
	}
	_ = s.DB.SetSessionState(ctx, row.ID, "closed", row.NextCursor)
	r := wire.Base(true, requestID, "closed")
	r["operation_id"] = row.ID
	return r, nil
}

func (s *Service) Attach(ctx context.Context, sess string, emit func(wire.Event)) error {
	ls, err := s.getLive(sess)
	if err != nil {
		return err
	}
	if ls.state != "open" {
		return wire.E("REMOTE_UNREACHABLE", "session is not open")
	}
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.attach[ls.id] = append(s.attach[ls.id], cancel)
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.attach[ls.id] = filterCancel(s.attach[ls.id], cancel)
		s.mu.Unlock()
	}()
	evs, _ := logstore.Replay(ls.w.Path(), 0)
	after := int64(0)
	for _, ev := range evs {
		emit(ev)
		after = ev.Cursor
	}
	ch, unsub := ls.w.Subscribe(128)
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if ev.Cursor > after {
				emit(ev)
				after = ev.Cursor
			}
		}
	}
}

func filterCancel(in []context.CancelFunc, drop context.CancelFunc) []context.CancelFunc {
	out := in[:0]
	for _, c := range in {
		if fmt.Sprintf("%p", c) == fmt.Sprintf("%p", drop) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (s *Service) Detach(ctx context.Context, requestID, sess string) (wire.Result, error) {
	_ = ctx
	id := sess
	if ls, err := s.getLive(sess); err == nil {
		id = ls.id
	}
	s.mu.Lock()
	cancels := s.attach[id]
	if len(cancels) == 0 {
		s.mu.Unlock()
		return nil, wire.E("INVALID_ARGUMENT", "no attach")
	}
	for _, c := range cancels {
		c()
	}
	delete(s.attach, id)
	s.mu.Unlock()
	r := wire.Base(true, requestID, "open")
	r["operation_id"] = id
	return r, nil
}

func (s *Service) cancelAttach(id string) {
	s.mu.Lock()
	for _, c := range s.attach[id] {
		c()
	}
	delete(s.attach, id)
	s.mu.Unlock()
}

func (s *Service) Monitor(ctx context.Context, requestID, target string, after int64, allowPublic bool, emit func(wire.Event)) error {
	ls, err := s.openSerial(ctx, target, allowPublic)
	if err != nil {
		return err
	}
	evs, _ := logstore.Replay(ls.w.Path(), after)
	cur := after
	for _, ev := range evs {
		emit(ev)
		if ev.Cursor > cur {
			cur = ev.Cursor
		}
	}
	ch, unsub := ls.w.Subscribe(128)
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if ev.Cursor > cur {
				emit(ev)
				cur = ev.Cursor
			}
		}
	}
}

func (s *Service) openSerial(ctx context.Context, target string, allowPublic bool) (*liveSess, error) {
	t, err := s.Resolve(ctx, target, allowPublic)
	if err != nil {
		return nil, err
	}
	if t.Transport != "serial" {
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "serial monitor requires a COM target")
	}
	name := t.Ref
	s.mu.Lock()
	if ex := s.byName[name]; ex != nil && ex.kind == "serial_repl" {
		s.mu.Unlock()
		return ex, nil
	}
	s.mu.Unlock()
	if row, _ := s.DB.GetActiveSessionByName(ctx, name); row != nil {
		s.mu.Lock()
		if ex := s.live[row.ID]; ex != nil {
			s.mu.Unlock()
			return ex, nil
		}
		s.mu.Unlock()
		ls, err := s.reattachSerial(ctx, row, t)
		if err == nil {
			return ls, nil
		}
	}
	entry, err := s.Pool.Acquire(ctx, t)
	if err != nil {
		return nil, err
	}
	sc, ok := entry.Transport.(contract.SerialConn)
	if !ok {
		s.Pool.Release(entry)
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "no serial stream")
	}
	id := wire.NewSessID()
	now := storage.NowUS()
	dir := filepath.Join(s.DataDir, "logs", "sessions", id)
	_ = os.MkdirAll(dir, 0o700)
	w, err := s.Logs.Open(dir, id, 0)
	if err != nil {
		s.Pool.Release(entry)
		return nil, err
	}
	row := &storage.Session{
		ID: id, Name: name, TargetRef: t.Ref, Kind: "serial_repl", State: "open",
		EventPath: w.Path(), CreatedAt: now, LastActivityAt: now, Version: 1,
	}
	if err := s.DB.InsertSession(ctx, row); err != nil {
		if wire.Is(err, "INVALID_ARGUMENT") {
			if existing, _ := s.DB.GetActiveSessionByName(ctx, name); existing != nil {
				s.Pool.Release(entry)
				return s.reattachSerial(ctx, existing, t)
			}
		}
		s.Pool.Release(entry)
		return nil, err
	}
	ls := &liveSess{id: id, name: name, kind: "serial_repl", state: "open", row: row, target: t, serial: sc, entry: entry, w: w}
	s.putLive(ls)
	go s.pumpSerial(ls)
	return ls, nil
}

func (s *Service) reattachSerial(ctx context.Context, row *storage.Session, t *config.Resolved) (*liveSess, error) {
	entry, err := s.Pool.Acquire(ctx, t)
	if err != nil {
		_ = s.DB.SetSessionState(ctx, row.ID, "disconnected", row.NextCursor)
		return nil, err
	}
	sc, ok := entry.Transport.(contract.SerialConn)
	if !ok {
		s.Pool.Release(entry)
		return nil, wire.E("CAPABILITY_UNSUPPORTED", "no serial stream")
	}
	dir := filepath.Dir(row.EventPath)
	w, err := s.Logs.Open(dir, row.ID, row.NextCursor)
	if err != nil {
		s.Pool.Release(entry)
		return nil, err
	}
	_, _ = w.Append("state", "", map[string]any{"state": "disconnected"})
	_, _ = w.Append("state", "", map[string]any{"state": "open"})
	row.State = "open"
	row.NextCursor = w.Cursor()
	_ = s.DB.SetSessionState(ctx, row.ID, "open", w.Cursor())
	ls := &liveSess{id: row.ID, name: row.Name, kind: row.Kind, state: "open", row: row, target: t, serial: sc, entry: entry, w: w}
	s.putLive(ls)
	go s.pumpSerial(ls)
	return ls, nil
}

func (s *Service) Recover(ctx context.Context) error {
	rows, err := s.DB.ListRecoverableSessions(ctx)
	if err != nil {
		return err
	}
	for i := range rows {
		row := rows[i]
		switch row.Kind {
		case "ssh_pty":
			_, _ = s.Logs.Open(filepath.Dir(row.EventPath), row.ID, row.NextCursor)
			_ = s.DB.SetSessionState(ctx, row.ID, "disconnected", row.NextCursor)
			_ = s.DB.SetSessionState(ctx, row.ID, "closed", row.NextCursor)
		case "serial_repl":
			t, rerr := s.Resolve(ctx, row.TargetRef, true)
			if rerr != nil || t.Transport != "serial" {
				_ = s.DB.SetSessionState(ctx, row.ID, "disconnected", row.NextCursor)
				continue
			}
			if _, err := s.reattachSerial(ctx, &row, t); err != nil {
				_ = s.DB.SetSessionState(ctx, row.ID, "disconnected", row.NextCursor)
			}
		default:
			_ = s.DB.SetSessionState(ctx, row.ID, "closed", row.NextCursor)
		}
	}
	return nil
}

func (s *Service) RecoverSSH() {
	_ = s.Recover(context.Background())
}

func (s *Service) Stdin(sess string) io.Writer {
	ls, err := s.getLive(sess)
	if err != nil {
		return nil
	}
	return ls.pty
}

func randNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
