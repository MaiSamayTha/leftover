package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

func put(t *testing.T, path string, size int) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testApp(t *testing.T) (*App, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a Linux layout")
	}
	home := t.TempDir()
	put(t, filepath.Join(home, "code/web/package.json"), 2)
	put(t, filepath.Join(home, "code/web/node_modules/a.js"), 40_000)
	put(t, filepath.Join(home, ".claude/projects/chat.jsonl"), 9_000)
	s := &scan.Scanner{
		Catalog: rules.MustLoad(),
		Host:    rules.Host{OS: "linux", Home: home, Getenv: func(string) string { return "" }},
	}
	a, err := OpenAt(filepath.Join(home, ".local/share/leftover"), s)
	if err != nil {
		t.Fatal(err)
	}
	return a, home
}

func item(t *testing.T, a *App, rule string) scan.Item {
	t.Helper()
	for _, it := range a.Last().Items {
		if it.Rule == rule {
			return it
		}
	}
	t.Fatalf("no %s item in the last scan", rule)
	return scan.Item{}
}

func TestScanCleanRestoreRoundTrip(t *testing.T) {
	a, home := testApp(t)
	if _, _, err := a.Scan(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	nm := item(t, a, "node-modules")
	out := a.Clean([]string{nm.ID})
	if len(out) != 1 || out[0].Error != "" || out[0].Entry == nil {
		t.Fatalf("clean failed: %+v", out)
	}
	if _, err := os.Stat(filepath.Join(home, "code/web/node_modules")); !os.IsNotExist(err) {
		t.Fatal("node_modules should be in the vault")
	}
	if _, ok := a.Find(nm.ID); ok {
		t.Fatal("cleaned items should drop out of the last scan")
	}
	// A fresh App reads the saved scan and sees the same thing.
	b, err := OpenAt(a.Dir, a.Scanner)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Find(nm.ID); ok {
		t.Fatal("saved scan should not list the cleaned item")
	}
	if _, _, err := b.Vault.Restore(out[0].Entry.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "code/web/node_modules/a.js")); err != nil {
		t.Fatal("restore didn't bring it back")
	}
}

func TestCleanRefusesYourData(t *testing.T) {
	a, home := testApp(t)
	a.Scan(context.Background(), nil)
	hist := item(t, a, "ai-agent-history")
	out := a.Clean([]string{hist.ID, "doesnotexist"})
	if out[0].Error == "" || out[1].Error == "" {
		t.Fatalf("both should fail: %+v", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude/projects/chat.jsonl")); err != nil {
		t.Fatal("your data must never move")
	}
}

func TestIgnoreDropsItemsAndPersists(t *testing.T) {
	a, home := testApp(t)
	a.Scan(context.Background(), nil)
	if err := a.Ignore(filepath.Join(home, "code")); err != nil {
		t.Fatal(err)
	}
	for _, it := range a.Last().Items {
		if it.Rule == "node-modules" {
			t.Fatal("ignored folder still listed")
		}
	}
	a.Scan(context.Background(), nil)
	for _, it := range a.Last().Items {
		if it.Rule == "node-modules" {
			t.Fatal("ignored folder found again by a new scan")
		}
	}
	b, _ := OpenAt(a.Dir, a.Scanner)
	if len(b.Settings.Ignore) != 1 {
		t.Fatal("ignore list wasn't saved")
	}
}

func TestExplain(t *testing.T) {
	a, home := testApp(t)
	r, at := a.Explain(filepath.Join(home, "code/web/node_modules/a.js"))
	if r == nil || r.ID != "node-modules" || at != filepath.Join(home, "code/web/node_modules") {
		t.Fatalf("got %v at %q", r, at)
	}
	if r, _ := a.Explain(filepath.Join(home, "code/web")); r != nil {
		t.Fatalf("a plain project folder shouldn't match a rule, got %s", r.ID)
	}
}

func TestExplainEditorExtensions(t *testing.T) {
	a, home := testApp(t)
	for _, base := range []string{".qoder/extensions/acme.tool-1.0.0", ".neweditor/extensions/acme.tool-1.0.0"} {
		put(t, filepath.Join(home, base, "package.json"), 2)
		put(t, filepath.Join(home, base, "node_modules/dep/index.js"), 2)
		r, _ := a.Explain(filepath.Join(home, base, "node_modules"))
		if r == nil || r.ID != "editor-extensions" || r.Verdict.Cleanable() {
			t.Fatalf("%s: got %v, want the editor-extensions rule, never cleanable", base, r)
		}
	}
}

func TestOnlyOneScanAtATime(t *testing.T) {
	a, _ := testApp(t)
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()
	if _, _, err := a.Scan(context.Background(), nil); err != ErrBusy {
		t.Fatalf("want ErrBusy, got %v", err)
	}
}
