package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeHome builds a small machine in a temp folder.
func fakeHome(t *testing.T) string {
	home := t.TempDir()
	// A JavaScript project.
	write(t, filepath.Join(home, "code/web/package.json"), 10)
	write(t, filepath.Join(home, "code/web/node_modules/react/index.js"), 50_000)
	write(t, filepath.Join(home, "code/web/.next/cache/x"), 20_000)
	// A node_modules with no package.json beside it isn't ours to judge.
	write(t, filepath.Join(home, "code/loose/node_modules/a.js"), 5_000)
	// A Rust project.
	write(t, filepath.Join(home, "code/rs/Cargo.toml"), 10)
	write(t, filepath.Join(home, "code/rs/target/CACHEDIR.TAG"), 10)
	write(t, filepath.Join(home, "code/rs/target/debug/app"), 80_000)
	// A folder called target that isn't Cargo's.
	write(t, filepath.Join(home, "code/notes/target/plan.txt"), 100)
	// Python with scattered bytecode and two environments.
	write(t, filepath.Join(home, "code/py/pyproject.toml"), 10)
	write(t, filepath.Join(home, "code/py/pkg/__pycache__/a.pyc"), 3_000)
	write(t, filepath.Join(home, "code/py/pkg/sub/__pycache__/b.pyc"), 3_000)
	write(t, filepath.Join(home, "code/py/.venv/pyvenv.cfg"), 0)
	os.WriteFile(filepath.Join(home, "code/py/.venv/pyvenv.cfg"), []byte("home = /definitely/not/here/bin\n"), 0o644)
	write(t, filepath.Join(home, "code/py/.venv/lib/site.py"), 30_000)
	write(t, filepath.Join(home, "scratch/venv/pyvenv.cfg"), 0)
	write(t, filepath.Join(home, "scratch/venv/lib/x.py"), 9_000)
	// Hugging Face models, listed one by one.
	write(t, filepath.Join(home, ".cache/huggingface/hub/models--a--one/blob"), 70_000)
	write(t, filepath.Join(home, ".cache/huggingface/hub/models--b--two/blob"), 40_000)
	// A folder the user asked us to ignore.
	write(t, filepath.Join(home, "private/app/package.json"), 10)
	write(t, filepath.Join(home, "private/app/node_modules/x.js"), 9_000)
	return home
}

func scanner(home string) *Scanner {
	return &Scanner{
		Catalog: rules.MustLoad(),
		Host:    rules.Host{OS: "linux", Home: home, Getenv: func(string) string { return "" }},
	}
}

func byRule(res *Result) map[string][]Item {
	m := map[string][]Item{}
	for _, it := range res.Items {
		m[it.Rule] = append(m[it.Rule], it)
	}
	return m
}

func TestScanFindsTheRightThings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Linux host layout")
	}
	home := fakeHome(t)
	res, err := scanner(home).Scan(context.Background(), Options{Ignore: []string{filepath.Join(home, "private")}})
	if err != nil {
		t.Fatal(err)
	}
	got := byRule(res)

	if n := len(got["node-modules"]); n != 1 {
		t.Fatalf("want 1 node_modules (loose and ignored ones excluded), got %d: %+v", n, got["node-modules"])
	}
	if p := got["node-modules"][0].Project; p != filepath.Join(home, "code/web") {
		t.Errorf("node_modules project = %q", p)
	}
	if len(got["next-build"]) != 1 {
		t.Error("missing .next")
	}
	if n := len(got["cargo-target"]); n != 1 {
		t.Errorf("want only the real Cargo target, got %d", n)
	}
	pyc := got["pycache"]
	if len(pyc) != 1 || len(pyc[0].Paths) != 2 {
		t.Fatalf("__pycache__ should be one grouped item with two folders, got %+v", pyc)
	}
	if pyc[0].Project != filepath.Join(home, "code/py") {
		t.Errorf("grouped project = %q", pyc[0].Project)
	}
	venv := got["python-venv"]
	if len(venv) != 1 || len(venv[0].Notes) == 0 {
		t.Errorf("listed venv with a missing interpreter should carry a note: %+v", venv)
	}
	if len(got["python-venv-unlisted"]) != 1 || got["python-venv-unlisted"][0].Verdict != rules.Review {
		t.Error("an unlisted venv must be found and only marked for review")
	}
	if n := len(got["hf-hub"]); n != 2 {
		t.Errorf("want each model separately, got %d", n)
	}
	for i := 1; i < len(res.Items); i++ {
		if res.Items[i].Bytes > res.Items[i-1].Bytes {
			t.Fatal("items must be sorted largest first")
		}
	}
	for _, it := range res.Items {
		if it.ID == "" || len(it.ID) != 12 {
			t.Errorf("bad id %q", it.ID)
		}
	}
}

func TestScanIsReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Linux host layout")
	}
	home := fakeHome(t)
	before := snapshot(t, home)
	if _, err := scanner(home).Scan(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, home)
	if len(before) != len(after) {
		t.Fatalf("scan changed the file tree: %d entries before, %d after", len(before), len(after))
	}
	for p, m := range before {
		if after[p] != m {
			t.Fatalf("scan touched %s", p)
		}
	}
}

func snapshot(t *testing.T, root string) map[string]time.Time {
	m := map[string]time.Time{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			m[p] = info.ModTime()
		}
		return nil
	})
	return m
}

func TestMeasureSkipsLinksAndCountsHardLinksOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	outside := t.TempDir()
	write(t, filepath.Join(outside, "huge"), 500_000)
	write(t, filepath.Join(dir, "a"), 10_000)
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(dir, "a"), filepath.Join(dir, "a-again")); err != nil {
		t.Fatal(err)
	}
	sz, err := NewSizer(2).Measure(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if sz.Files != 2 {
		t.Errorf("want 2 files (the link is not followed), got %d", sz.Files)
	}
	if sz.Logical != 20_000 {
		t.Errorf("logical size should count both names: %d", sz.Logical)
	}
	if sz.Bytes > 16_384 {
		t.Errorf("on-disk size should count the hard-linked file once, got %d", sz.Bytes)
	}
}

func TestScanCancels(t *testing.T) {
	home := fakeHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanner(home).Scan(ctx, Options{}); err == nil {
		t.Fatal("a cancelled scan should return an error")
	}
}

func TestMinAge(t *testing.T) {
	home := t.TempDir()
	temp := filepath.Join(home, "AppData", "Local", "Temp")
	write(t, filepath.Join(temp, "old/file"), 4096)
	write(t, filepath.Join(temp, "fresh/file"), 4096)
	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(filepath.Join(temp, "old/file"), old, old)
	os.Chtimes(filepath.Join(temp, "old"), old, old)

	s := &Scanner{Catalog: rules.MustLoad(), Host: rules.Host{OS: "windows", Home: home, Getenv: func(k string) string {
		if k == "TEMP" {
			return temp
		}
		return ""
	}}}
	if runtime.GOOS != "windows" {
		// Windows paths built on Unix use backslashes, which don't exist here.
		t.Skip("needs a real Windows file system")
	}
	res, err := s.Scan(context.Background(), Options{SkipProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	items := byRule(res)["user-temp"]
	if len(items) != 1 || filepath.Base(items[0].Paths[0]) != "old" {
		t.Fatalf("only the old temp entry should be reported, got %+v", items)
	}
}
