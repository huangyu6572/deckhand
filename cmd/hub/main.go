package main

import (
	"errors"
	"fmt"
	"os"

	"localaihub/internal/cli"
	"localaihub/internal/wire"
)

func main() {
	if err := cli.RejectLeadingFlags(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	if err := cli.Root().Execute(); err != nil {
		var e *wire.Error
		if errors.As(err, &e) {
			os.Exit(wire.ExitCode(e.Code))
		}
		os.Exit(1)
	}
}
