package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

// listProcesses returns the command line of every running program. It is
// swapped out in tests. An error means Leftover couldn't tell, and nothing
// is held back because of it.
var listProcesses = func(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "linux":
		return procCmdlines()
	case "windows":
		out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
			"Get-CimInstance Win32_Process | ForEach-Object { if ($_.CommandLine) { $_.CommandLine } else { $_.Name } }").Output()
		if err == nil {
			return lines(out), nil
		}
		// Without PowerShell, image names are still enough for most checks.
		out, err = exec.CommandContext(ctx, "tasklist", "/fo", "csv", "/nh").Output()
		if err != nil {
			return nil, err
		}
		recs, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
		if err != nil {
			return nil, err
		}
		var names []string
		for _, r := range recs {
			if len(r) > 0 {
				names = append(names, r[0])
			}
		}
		return names, nil
	default:
		out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "command=").Output()
		if err != nil {
			return nil, err
		}
		return lines(out), nil
	}
}

func procCmdlines() ([]string, error) {
	dirs, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range dirs {
		raw, err := os.ReadFile(p)
		if err != nil || len(raw) == 0 {
			continue
		}
		out = append(out, strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", " ")))
	}
	return out, nil
}

func lines(b []byte) []string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// busyChecker answers "is something using this rule's files right now",
// listing processes at most once.
type busyChecker struct {
	ctx    context.Context
	listed bool
	procs  []string
}

// check returns the first running program a rule says uses its files.
func (b *busyChecker) check(r *rules.Rule) (rules.Busy, bool) {
	if len(r.Busy) == 0 {
		return rules.Busy{}, false
	}
	if !b.listed {
		b.listed = true
		b.procs, _ = listProcesses(b.ctx)
	}
	for _, bz := range r.Busy {
		for _, cmd := range b.procs {
			if runningAs(cmd, bz) {
				return bz, true
			}
		}
	}
	return rules.Busy{}, false
}

var (
	patMu        sync.Mutex
	procPatterns = map[string]*regexp.Regexp{}
)

// runningAs reports whether a command line belongs to the program bz
// describes: its executable is one of bz.Process (in any folder, with or
// without .exe, ignoring case), or the line contains one of bz.Args.
func runningAs(cmd string, bz rules.Busy) bool {
	for _, a := range bz.Args {
		if strings.Contains(cmd, a) {
			return true
		}
	}
	for _, name := range bz.Process {
		patMu.Lock()
		re, ok := procPatterns[name]
		if !ok {
			re = regexp.MustCompile(`(?i)(^|[/\\"])` + regexp.QuoteMeta(name) + `(\.exe)?("|\s|$)`)
			procPatterns[name] = re
		}
		patMu.Unlock()
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}

// BusyMessage explains why an item wasn't moved.
func BusyMessage(bz rules.Busy) string {
	msg := bz.Name + " is running and may be using these files."
	if bz.Stop != "" {
		msg += " " + bz.Stop
	}
	return msg + " Then try again."
}
