package cli

import "testing"

func TestRejectFlagsBeforeVerb(t *testing.T) {
	if err := rejectFlagsBeforeVerb([]string{"--json", "run", "t"}); err == nil {
		t.Fatal("expected error")
	}
	if err := rejectFlagsBeforeVerb([]string{"run", "--json", "t"}); err != nil {
		t.Fatal(err)
	}
	if err := rejectFlagsBeforeVerb([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
}

func TestRejectPasswordArgv(t *testing.T) {
	if err := rejectPasswordArgv([]string{"run", "t", "--password", "secret"}); err == nil {
		t.Fatal("expected error")
	}
	if err := rejectPasswordArgv([]string{"run", "t", "--password=secret"}); err == nil {
		t.Fatal("expected error")
	}
	if err := rejectPasswordArgv([]string{"run", "t", "--", "echo", "--password"}); err != nil {
		t.Fatal(err)
	}
}
