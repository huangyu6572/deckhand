package job

import (
	"testing"

	"localaihub/internal/wire"
)

func TestCompletedMessageFromEvents(t *testing.T) {
	evs := []wire.Event{
		{Type: "started"},
		{Type: "completed", ErrorCode: "AUTH_FAILED", Message: "authentication failed for u@h; offered keys: SHA256:abc"},
	}
	got := completedMessage(evs)
	if got == "" || got == "AUTH_FAILED" {
		t.Fatalf("got %q", got)
	}
	if completedMessage(nil) != "" {
		t.Fatal("empty events")
	}
}
