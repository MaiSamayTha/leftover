package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
)

// Exit codes, documented in the README so scripts, cron jobs and CI can
// rely on them.
const (
	exitOK          = 0   // done, including "nothing to clear"
	exitError       = 1   // something went wrong
	exitUsage       = 2   // bad command, flag or argument
	exitPartial     = 3   // some items couldn't be moved or restored
	exitInterrupted = 130 // stopped with Ctrl-C
)

// exitErr carries the exit code an error should end the program with.
type exitErr struct {
	code int
	err  error
}

func (e *exitErr) Error() string { return e.err.Error() }
func (e *exitErr) Unwrap() error { return e.err }

func usageError(format string, a ...any) error {
	return &exitErr{exitUsage, fmt.Errorf(format, a...)}
}

func partialError(msg string) error {
	return &exitErr{exitPartial, errors.New(msg)}
}

// exitCode picks the code for the error run returned.
func exitCode(err error) int {
	var e *exitErr
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return exitOK
	case errors.As(err, &e):
		return e.code
	case errors.Is(err, context.Canceled):
		return exitInterrupted
	}
	return exitError
}

// flagSet is a flag.FlagSet whose parse errors exit with exitUsage.
type flagSet struct{ *flag.FlagSet }

func (f flagSet) Parse(args []string) error {
	err := f.FlagSet.Parse(args)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return err
	}
	return &exitErr{exitUsage, err}
}
