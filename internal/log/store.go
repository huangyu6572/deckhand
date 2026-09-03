package logstore

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"localaihub/internal/wire"
)

type Store struct {
	mu     sync.Mutex
	maxEvt int
	maxOp  int
}

func New(maxEvent, maxOp int) *Store {
	if maxEvent <= 0 {
		maxEvent = 262144
	}
	if maxOp <= 0 {
		maxOp = 104857600
	}
	return &Store{maxEvt: maxEvent, maxOp: maxOp}
}

type Writer struct {
	store  *Store
	path   string
	opID   string
	cursor int64
	size   int64
	subs   []chan wire.Event
	mu     sync.Mutex
	closed bool
}

func (s *Store) Open(dir, opID string, startCursor int64) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "events.jsonl")
	st, _ := os.Stat(path)
	var size int64
	if st != nil {
		size = st.Size()
	}
	if startCursor == 0 && size > 0 {
		startCursor = MaxCursor(path)
	} else if startCursor > 0 {
		if m := MaxCursor(path); m > startCursor {
			startCursor = m
		}
	}
	return &Writer{store: s, path: path, opID: opID, cursor: startCursor, size: size}, nil
}

func (w *Writer) Path() string { return w.path }

func (w *Writer) Cursor() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cursor
}

func (w *Writer) Subscribe(buf int) (<-chan wire.Event, func()) {
	ch := make(chan wire.Event, buf)
	w.mu.Lock()
	w.subs = append(w.subs, ch)
	w.mu.Unlock()
	return ch, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for i, s := range w.subs {
			if s == ch {
				w.subs = append(w.subs[:i], w.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

func (w *Writer) Append(typ string, data string, extra map[string]any) (wire.Event, error) {
	chunks := split(data, w.store.maxEvt)
	if len(chunks) == 0 {
		chunks = []string{""}
	}
	var last wire.Event
	for _, c := range chunks {
		ev, err := w.appendOne(typ, c, extra)
		if err != nil {
			return last, err
		}
		last = ev
	}
	return last, nil
}

func (w *Writer) appendOne(typ, data string, extra map[string]any) (wire.Event, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size > int64(w.store.maxOp) && typ != "completed" && typ != "started" && typ != "state" {
		return wire.Event{}, wire.E("OUTPUT_LIMIT_EXCEEDED", "operation log exceeded max_operation_log_bytes")
	}
	w.cursor++
	ev := wire.Event{
		Type:        typ,
		OperationID: w.opID,
		Cursor:      w.cursor,
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if utf8.ValidString(data) {
		ev.Data = data
	} else if data != "" {
		ev.DataBase64 = base64.StdEncoding.EncodeToString([]byte(data))
	} else {
		ev.Data = data
	}
	b, err := json.Marshal(mergeEvent(ev, extra))
	if err != nil {
		w.cursor--
		return ev, err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		w.cursor--
		return ev, wire.E("OUTPUT_PERSIST_FAILED", err.Error())
	}
	n, err := f.Write(append(b, '\n'))
	if err != nil {
		f.Close()
		w.cursor--
		return ev, wire.E("OUTPUT_PERSIST_FAILED", err.Error())
	}
	sync := typ == "started" || typ == "completed" || typ == "state"
	if sync {
		if err := f.Sync(); err != nil {
			f.Close()
			w.cursor--
			return ev, wire.E("OUTPUT_PERSIST_FAILED", err.Error())
		}
	}
	if err := f.Close(); err != nil {
		w.cursor--
		return ev, wire.E("OUTPUT_PERSIST_FAILED", err.Error())
	}
	w.size += int64(n)
	for _, sub := range w.subs {
		select {
		case sub <- ev:
		default:
			// drop on this subscriber; job continues
		}
	}
	return ev, nil
}

func mergeEvent(ev wire.Event, extra map[string]any) map[string]any {
	m := map[string]any{
		"type":         ev.Type,
		"operation_id": ev.OperationID,
		"cursor":       ev.Cursor,
		"timestamp":    ev.Timestamp,
	}
	if ev.Data != "" {
		m["data"] = ev.Data
	}
	if ev.DataBase64 != "" {
		m["data_base64"] = ev.DataBase64
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func split(s string, max int) []string {
	if max <= 0 || len(s) <= max {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	var out []string
	b := []byte(s)
	for len(b) > 0 {
		n := max
		if n > len(b) {
			n = len(b)
		}
		for n > 0 && !utf8.Valid(b[:n]) {
			n--
		}
		if n == 0 {
			n = 1
		}
		out = append(out, string(b[:n]))
		b = b[n:]
	}
	return out
}

func Replay(path string, after int64) ([]wire.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []wire.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var ev wire.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Cursor > after {
			out = append(out, ev)
		}
	}
	return out, sc.Err()
}

func MaxCursor(path string) int64 {
	first, last, _ := Bounds(path)
	_ = first
	return last
}

func Bounds(path string) (first, last int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var ev wire.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Cursor <= 0 {
			continue
		}
		if first == 0 || ev.Cursor < first {
			first = ev.Cursor
		}
		if ev.Cursor > last {
			last = ev.Cursor
		}
	}
	return first, last, sc.Err()
}

func EventBytes(ev wire.Event) string {
	if ev.DataBase64 != "" {
		b, err := base64.StdEncoding.DecodeString(ev.DataBase64)
		if err == nil {
			return string(b)
		}
	}
	return ev.Data
}

func CollectStd(events []wire.Event) (stdout, stderr string) {
	for _, ev := range events {
		switch ev.Type {
		case "stdout":
			stdout += ev.Data
		case "stderr":
			stderr += ev.Data
		}
	}
	return stdout, stderr
}
