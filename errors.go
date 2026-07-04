package main

import (
	"fmt"
	"os"
)

type cliError struct{ msg string }

func (e cliError) Error() string { return e.msg }

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
