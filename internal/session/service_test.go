package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"localaihub/internal/log"
	"localaihub/internal/storage"
)

func TestReadReportsGapAndEOF(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	logs := logstore.New(1024, 1<<20)
	s := New(dir, db, logs, nil, nil)
	id := "sess_gap"
	sessDir := filepath.Join(dir, "logs", "sessions", id)
	os.MkdirAll(sessDir, 0o700)
	path := filepath.Join(sessDir, "events.jsonl")
	for _, c := range []int64{4, 5} {
		ev := map[string]any{"type": "stdout", "operation_id": id, "cursor": c, "timestamp": "t", "data": "z"}
		b, _ := json.Marshal(ev)
		f, _ := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		f.Write(append(b, '\n'))
		f.Close()
	}
	now := storage.NowUS()
	row := &storage.Session{
		ID: id, Name: id, TargetRef: "dev-web", Kind: "ssh_pty", State: "closed",
		EventPath: path, CreatedAt: now, LastActivityAt: now, Version: 1,
	}
	if err := db.InsertSession(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	r, err := s.Read(context.Background(), "req_1", id, 0, 0, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if r["eof"] != true {
		t.Fatalf("eof %+v", r)
	}
	if r["first_available_cursor"] != int64(4) {
		t.Fatalf("first %+v", r["first_available_cursor"])
	}
	if r["events_lost"] != int64(3) {
		t.Fatalf("lost %+v", r["events_lost"])
	}
	if r["data"] != "zz" {
		t.Fatalf("data %q", r["data"])
	}
}

func TestParseSentinelIgnoresPrintfEcho(t *testing.T) {
	nonce := "aabbccdd"
	echo := "eval \"$(printf '%s' ABC | base64 -d)\"; printf '\\n__HUB_DONE_" + nonce + "_%s__\\n' \"$?\"\n"
	body := "hello\nworld\n"
	mark := "__HUB_DONE_" + nonce + "_0__\n"
	code, out, found := parseSentinel(echo+body+mark, nonce)
	if !found {
		t.Fatal("expected marker")
	}
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if out != "hello\nworld\n" {
		t.Fatalf("stdout %q", out)
	}
}

func TestParseSentinelReadsNonzero(t *testing.T) {
	nonce := "deadbeef"
	text := "printf '__HUB_DONE_" + nonce + "_%s__'\nfail\n__HUB_DONE_" + nonce + "_1__\n"
	code, out, found := parseSentinel(text, nonce)
	if !found || code != 1 {
		t.Fatalf("found=%v code=%d", found, code)
	}
	if !strings.Contains(out, "fail") {
		t.Fatalf("out %q", out)
	}
}

func TestParseSentinelIgnoresWrongNonce(t *testing.T) {
	_, _, found := parseSentinel("x\n__HUB_DONE_0__\n", "aabb")
	if found {
		t.Fatal("forged marker without nonce must not finish")
	}
}

func TestTakeWorkdirOnce(t *testing.T) {
	ls := &liveSess{}
	wd, cd, err := ls.takeWorkdir("~/sub-2-api")
	if err != nil || wd != "~/sub-2-api" || !cd {
		t.Fatalf("first enter wd=%q cd=%v err=%v", wd, cd, err)
	}
	ls.markEntered()
	wd, cd, err = ls.takeWorkdir("")
	if err != nil || wd != "~/sub-2-api" || cd {
		t.Fatalf("stay wd=%q cd=%v err=%v", wd, cd, err)
	}
	wd, cd, err = ls.takeWorkdir("~/sub-2-api")
	if err != nil || cd {
		t.Fatalf("same dir must not cd again cd=%v err=%v", cd, err)
	}
	ls.clearWorkdir()
	wd, cd, err = ls.takeWorkdir("")
	if err != nil || wd != "" || cd {
		t.Fatalf("after leave wd=%q cd=%v err=%v", wd, cd, err)
	}
}

func TestRecoverClosesSSHSession(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	logs := logstore.New(1024, 1<<20)
	s := New(dir, db, logs, nil, nil)
	now := storage.NowUS()
	path := filepath.Join(dir, "logs", "sessions", "sess_x", "events.jsonl")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, nil, 0o600)
	row := &storage.Session{
		ID: "sess_x", Name: "n1", TargetRef: "dev-web", Kind: "ssh_pty", State: "open",
		EventPath: path, CreatedAt: now, LastActivityAt: now, Version: 1,
	}
	if err := db.InsertSession(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), "sess_x")
	if err != nil || got == nil || got.State != "closed" {
		t.Fatalf("%+v %v", got, err)
	}
}
