package logstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReplayAndBoundsGap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	write := func(cursor int64) {
		ev := map[string]any{"type": "stdout", "operation_id": "sess_1", "cursor": cursor, "timestamp": "t", "data": "x"}
		b, _ := json.Marshal(ev)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(append(b, '\n'))
		f.Close()
	}
	write(3)
	write(4)
	write(5)
	first, last, err := Bounds(path)
	if err != nil || first != 3 || last != 5 {
		t.Fatalf("bounds %d %d %v", first, last, err)
	}
	if MaxCursor(path) != 5 {
		t.Fatalf("max %d", MaxCursor(path))
	}
	evs, err := Replay(path, 3)
	if err != nil || len(evs) != 2 || evs[0].Cursor != 4 {
		t.Fatalf("replay %+v %v", evs, err)
	}
}

func TestOpenResumesCursor(t *testing.T) {
	dir := t.TempDir()
	s := New(1024, 1<<20)
	w, err := s.Open(dir, "sess_1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Append("stdout", "a", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Append("stdout", "b", nil); err != nil {
		t.Fatal(err)
	}
	w2, err := s.Open(dir, "sess_1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if w2.Cursor() != 2 {
		t.Fatalf("cursor %d", w2.Cursor())
	}
	ev, err := w2.Append("stdout", "c", nil)
	if err != nil || ev.Cursor != 3 {
		t.Fatalf("%+v %v", ev, err)
	}
}

func TestAppendCompletedKeepsMessage(t *testing.T) {
	dir := t.TempDir()
	s := New(1024, 1<<20)
	w, err := s.Open(dir, "job_1", 0)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := w.Append("completed", "", map[string]any{
		"state":      "failed",
		"error_code": "AUTH_FAILED",
		"message":    "authentication failed for u@h; offered keys: SHA256:abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Message == "" || ev.ErrorCode != "AUTH_FAILED" {
		t.Fatalf("in-memory event missing extra: %+v", ev)
	}
	replayed, err := Replay(w.Path(), 0)
	if err != nil || len(replayed) != 1 {
		t.Fatalf("replay %v %v", replayed, err)
	}
	if replayed[0].Message != ev.Message {
		t.Fatalf("disk message %q", replayed[0].Message)
	}
}
