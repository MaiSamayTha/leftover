package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"":       0,
		"0":      0,
		"500":    500,
		"500B":   500,
		"1KB":    1e3,
		"1.5MB":  1.5e6,
		"500mb":  500e6,
		"2G":     2e9,
		"2 GB":   2e9,
		" 1TB  ": 1e12,
	}
	for in, want := range cases {
		got, err := parseSize(in)
		if err != nil || got != want {
			t.Errorf("parseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"lots", "-1MB", "MB"} {
		if _, err := parseSize(bad); err == nil {
			t.Errorf("parseSize(%q) should fail", bad)
		}
	}
}

func item(v rules.Verdict, bytes int64) scan.Item {
	it := scan.Item{Rule: "node-modules", Tool: "npm, Yarn, pnpm, Bun", Verdict: v, Paths: []string{"x"}}
	it.Bytes = bytes
	return it
}

func TestFilterKeepsByIdleAndTool(t *testing.T) {
	recent, old := item(rules.Rebuild, 5e6), item(rules.Rebuild, 5e6)
	recent.IdleDays, old.IdleDays = 3, 120

	f := filter{idle: 90}
	if f.keep(recent) || !f.keep(old) {
		t.Error("--idle 90 should keep only projects untouched for 90 days")
	}

	if !(filter{tool: "YARN"}).keep(old) {
		t.Error("--tool should match the tool name, ignoring case")
	}
	if !(filter{tool: "node-mod"}).keep(old) {
		t.Error("--tool should also match the rule id")
	}
	if (filter{tool: "cargo"}).keep(old) {
		t.Error("--tool cargo must not match an npm item")
	}
}

func TestFilterHidesSmallItemsLeftoverWontClean(t *testing.T) {
	small := item(rules.Review, 50e6)
	if (filter{}).keep(small) {
		t.Error("a small 'worth a look' item is noise without --all")
	}
	if !(filter{all: true}).keep(small) {
		t.Error("--all should show it")
	}
	if !(filter{}).keep(item(rules.Review, 200e6)) {
		t.Error("big 'worth a look' items are always shown")
	}
}

// fakeHome points Leftover at a throwaway home and data folder with one
// project's node_modules in it, and returns that folder's path.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(k, home)
	}
	t.Setenv("LEFTOVER_HOME", filepath.Join(home, "leftover-data"))
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	// The test's home is itself inside the real temp folder, which is a
	// cache Leftover knows on Windows; point TEMP elsewhere so the home
	// isn't treated as part of it and skipped.
	t.Setenv("TEMP", filepath.Join(home, "AppData", "Local", "Temp"))
	t.Setenv("TMP", filepath.Join(home, "AppData", "Local", "Temp"))
	t.Setenv("NO_COLOR", "1")

	web := filepath.Join(home, "code", "web")
	nm := filepath.Join(web, "node_modules", "react")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(web, "package.json"), []byte("{}"), 0o644)
	if err := os.WriteFile(filepath.Join(nm, "index.js"), make([]byte, 2e6), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(web, "node_modules")
}

// noTerminal makes stdin a pipe, as when Leftover runs from a script.
func noTerminal(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; r.Close() })
}

func TestCleanNeverRunsUnattendedWithoutYes(t *testing.T) {
	nm := fakeHome(t)
	noTerminal(t)

	out, err := captureRunStdout(t, func() error { return run([]string{"clean"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "node_modules") {
		t.Fatalf("the plan should list node_modules:\n%s", out)
	}
	if !strings.Contains(out, "Nothing moved.") {
		t.Fatalf("without a terminal or --yes, clean must not move anything:\n%s", out)
	}
	if _, err := os.Stat(nm); err != nil {
		t.Fatal("node_modules was moved without being asked")
	}

	if _, err := captureRunStdout(t, func() error { return run([]string{"clean", "--json"}) }); err == nil {
		t.Fatal("--json without --yes must be refused")
	}

	if _, err := captureRunStdout(t, func() error { return run([]string{"clean", "--yes"}) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(nm); err == nil {
		t.Fatal("clean --yes should have moved node_modules to the vault")
	}
}

func TestScanJSONListsItems(t *testing.T) {
	fakeHome(t)
	out, err := captureRunStdout(t, func() error { return run([]string{"scan", "-q", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"items"`) || !strings.Contains(out, "node_modules") {
		t.Fatalf("scan --json should include node_modules:\n%s", out)
	}
}

func TestBadFlagValuesAreErrors(t *testing.T) {
	fakeHome(t)
	for _, args := range [][]string{
		{"scan", "--min", "lots"},
		{"scan", "--idle", "soon"},
		{"clean", "--min", "lots", "--yes"},
	} {
		if _, err := captureRunStdout(t, func() error { return run(args) }); err == nil {
			t.Errorf("leftover %s should fail", strings.Join(args, " "))
		}
	}
}
