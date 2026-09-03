package storage

import (
	"context"
	"testing"
)

func TestSessionNameActiveUnique(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	now := NowUS()
	a := &Session{ID: "sess_a", Name: "dev", TargetRef: "dev-web", Kind: "ssh_pty", State: "open", EventPath: "p", CreatedAt: now, LastActivityAt: now, Version: 1}
	if err := db.InsertSession(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := &Session{ID: "sess_b", Name: "dev", TargetRef: "dev-web", Kind: "ssh_pty", State: "opening", EventPath: "q", CreatedAt: now, LastActivityAt: now, Version: 1}
	if err := db.InsertSession(ctx, b); err == nil {
		t.Fatal("expected unique name")
	}
	got, err := db.GetActiveSessionByName(ctx, "dev")
	if err != nil || got == nil || got.ID != "sess_a" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := db.SetSessionState(ctx, "sess_a", "closed", 0); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSession(ctx, b); err != nil {
		t.Fatal(err)
	}
}

func TestJobCannotLeaveSucceeded(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	op := &Operation{
		ID: "job_1", RequestID: "req_1", Kind: "run", TargetRef: "t", ResolvedJSON: "{}",
		State: "queued", EventPath: "e", Version: 1, CreatedAt: NowUS(),
	}
	if err := db.InsertOperation(ctx, op); err != nil {
		t.Fatal(err)
	}
	zero := 0
	if err := db.Finish(ctx, op.ID, 1, "succeeded", "", &zero, 0, false); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateRunning(ctx, op.ID, 2, NowUS(), NowUS(), 0); err == nil {
		t.Fatal("expected cas conflict")
	}
	got, err := db.Get(ctx, op.ID)
	if err != nil || got.State != "succeeded" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestDuplicateRequestID(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	op := &Operation{
		ID: "job_1", RequestID: "req_dup", Kind: "run", TargetRef: "t", ResolvedJSON: "{}",
		State: "queued", EventPath: "e", Version: 1, CreatedAt: NowUS(),
	}
	if err := db.InsertOperation(ctx, op); err != nil {
		t.Fatal(err)
	}
	op2 := *op
	op2.ID = "job_2"
	if err := db.InsertOperation(ctx, &op2); err == nil {
		t.Fatal("expected unique request_id")
	}
}
