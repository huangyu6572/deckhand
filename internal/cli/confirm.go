package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"localaihub/internal/destructive"
	"localaihub/internal/wire"
)

var (
	confirmIsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	confirmIn    io.Reader = os.Stdin
	confirmErr   io.Writer = os.Stderr
)

func confirmIfRm(target, inspect string) error {
	if !destructive.LooksLikeRm(inspect) {
		return nil
	}
	if !confirmIsTTY() {
		return destructive.RefuseUnlessConfirmed(inspect, false)
	}
	phrase := "DELETE " + strings.TrimSpace(target)
	fmt.Fprint(confirmErr, "hub: this would run rm on the remote host. Agents cannot confirm it.\n\n")
	fmt.Fprintf(confirmErr, "Target: %s\nCommand:\n-----\n%s\n-----\n\n", target, strings.TrimRight(inspect, "\n"))
	fmt.Fprintf(confirmErr, "A person at this keyboard must type: %s\n> ", phrase)
	line, err := bufio.NewReader(confirmIn).ReadString('\n')
	if err != nil && len(strings.TrimSpace(line)) == 0 {
		return wire.E("DESTROY_NEEDS_HUMAN", "confirmation not entered")
	}
	if strings.TrimSpace(line) != phrase {
		return wire.E("DESTROY_NEEDS_HUMAN", "confirmation did not match; nothing was sent")
	}
	return nil
}
