package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"localaihub/internal/wire"
)

func readSecret() (string, error) {
	fmt.Fprint(os.Stderr, "Secret: ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", wire.E("INVALID_ARGUMENT", "failed to read secret")
	}
	s := strings.TrimRight(string(b), "\r\n")
	if s == "" {
		return "", wire.E("INVALID_ARGUMENT", "empty secret")
	}
	return s, nil
}
