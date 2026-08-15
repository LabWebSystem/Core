package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	app, err := newApplication()
	if err != nil {
		fail(err)
	}
	if err := app.run(os.Args[1:]); err != nil {
		fail(err)
	}
}

type exitError struct{ code int }

func (e exitError) Error() string { return "" }

func fail(err error) {
	var exit exitError
	if errors.As(err, &exit) {
		os.Exit(exit.code)
	}
	fmt.Fprintf(os.Stderr, "lwsctl: %s\n", err)
	os.Exit(1)
}
