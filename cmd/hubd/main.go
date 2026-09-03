package main

import (
	"fmt"
	"os"
	"os/signal"

	"localaihub/internal/app"
)

func main() {
	a, err := app.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer a.Close()
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		<-ch
		a.Close()
		os.Exit(0)
	}()
	if err := a.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
