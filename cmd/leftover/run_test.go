package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureRunStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	// Read while fn runs: a pipe only buffers a few KB (less on Windows),
	// so a command with more output than that would block forever.
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := io.ReadAll(r)
		done <- result{b, err}
	}()

	runErr := fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	res := <-done
	if res.err != nil {
		t.Fatal(res.err)
	}
	return string(res.b), runErr
}

func TestRunHelp(t *testing.T) {
	out, err := captureRunStdout(t, func() error { return run([]string{"help"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "leftover scan") {
		t.Fatalf("unexpected help output:\n%s", out)
	}
}

func TestRunVersion(t *testing.T) {
	out, err := captureRunStdout(t, func() error { return run([]string{"--version"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "leftover "+version) {
		t.Fatalf("unexpected version output: %q", out)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	_, err := captureRunStdout(t, func() error { return run([]string{"nope"}) })
	if err == nil || !strings.Contains(err.Error(), `unknown command "nope"`) {
		t.Fatalf("got %v, want unknown command error", err)
	}
}

func TestRunWithoutArgsOpensApp(t *testing.T) {
	old := openApp
	t.Cleanup(func() { openApp = old })

	called := false
	openApp = func([]string) error {
		called = true
		return nil
	}

	if _, err := captureRunStdout(t, func() error { return run(nil) }); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("run with no args should open the app")
	}
}

func TestRunFromFinderOpensApp(t *testing.T) {
	old := openApp
	t.Cleanup(func() { openApp = old })
	called := false
	openApp = func([]string) error { called = true; return nil }

	if _, err := captureRunStdout(t, func() error { return run([]string{"-psn_0_12345"}) }); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("opening Leftover.app from Finder should open the window")
	}
}
