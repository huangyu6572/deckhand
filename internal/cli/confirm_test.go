package cli

import (
	"strings"
	"testing"

	"localaihub/internal/wire"
)

func TestConfirmIfRmSkipsSafeCommand(t *testing.T) {
	if err := confirmIfRm("cloud-172", "uname -a"); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmIfRmRequiresTTY(t *testing.T) {
	oldTTY, oldIn := confirmIsTTY, confirmIn
	defer func() { confirmIsTTY, confirmIn = oldTTY, oldIn }()
	confirmIsTTY = func() bool { return false }
	err := confirmIfRm("cloud-172", "rm -rf /tmp/x")
	if err == nil || !wire.Is(err, "DESTROY_NEEDS_HUMAN") {
		t.Fatalf("got %v", err)
	}
}

func TestConfirmIfRmAcceptsPhraseOnTTY(t *testing.T) {
	oldTTY, oldIn := confirmIsTTY, confirmIn
	defer func() { confirmIsTTY, confirmIn = oldTTY, oldIn }()
	confirmIsTTY = func() bool { return true }
	confirmIn = strings.NewReader("DELETE cloud-172\n")
	if err := confirmIfRm("cloud-172", "rm -rf /tmp/x"); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmIfRmRejectsWrongPhrase(t *testing.T) {
	oldTTY, oldIn := confirmIsTTY, confirmIn
	defer func() { confirmIsTTY, confirmIn = oldTTY, oldIn }()
	confirmIsTTY = func() bool { return true }
	confirmIn = strings.NewReader("yes\n")
	err := confirmIfRm("cloud-172", "rm -rf /tmp/x")
	if err == nil || !wire.Is(err, "DESTROY_NEEDS_HUMAN") {
		t.Fatalf("got %v", err)
	}
}
