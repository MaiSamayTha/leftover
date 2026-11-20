package main

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Every flag offered for completion must be a real flag of that command.
func TestCompletionFlagsExist(t *testing.T) {
	fakeHome(t)
	for key, flags := range completionFlags {
		r, w, _ := os.Pipe()
		old := os.Stderr
		os.Stderr = w
		_ = run(append(strings.Fields(key), "-h"))
		os.Stderr = old
		w.Close()
		help, _ := io.ReadAll(r)
		for _, f := range flags {
			if !strings.Contains(string(help), "  -"+strings.TrimLeft(f, "-")) {
				t.Errorf("leftover %s has no flag %s:\n%s", key, f, help)
			}
		}
	}
}

func TestComplete(t *testing.T) {
	fakeHome(t)
	for _, c := range []struct {
		cur   string
		prior []string
		want  []string
	}{
		{"va", nil, []string{"vault"}},
		{"", []string{"vault"}, []string{"empty", "list", "restore"}},
		{"--l", []string{"vault", "restore"}, []string{"--last"}},
		{"--m", []string{"clean"}, []string{"--min"}},
		{"", []string{"scan", "--min"}, nil},
		{"", []string{"explore"}, []string{completeDirs}},
		{"z", []string{"completion"}, []string{"zsh"}},
	} {
		if got := complete(c.cur, c.prior); !reflect.DeepEqual(got, c.want) {
			t.Errorf("complete(%q, %v) = %v, want %v", c.cur, c.prior, got, c.want)
		}
	}
	for _, sh := range []string{"bash", "zsh", "fish", "powershell"} {
		out, err := captureRunStdout(t, func() error { return run([]string{"completion", sh}) })
		if err != nil || !strings.Contains(out, "__complete") {
			t.Errorf("completion %s: %v\n%s", sh, err, out)
		}
	}
	if exitCode(run([]string{"completion", "tcsh"})) != exitUsage {
		t.Error("an unknown shell is a usage error")
	}
}
