package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
		Host: rules.Host{OS: "linux", Home: home, Getenv: func(k string) string {
			// Keep {temp} inside the fake home, never the real /tmp.
			if k == "TMPDIR" || k == "LEFTOVER_TMP" {
				return filepath.Join(home, "tmp")
			}
			return ""
		}},
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

func TestDotnetBuildFoldersGroup(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "src/Api/Api.csproj"), 10)
	write(t, filepath.Join(home, "src/Api/bin/Debug/api.dll"), 9_000)
	write(t, filepath.Join(home, "src/Api/obj/project.assets.json"), 4_000)
	write(t, filepath.Join(home, "src/scripts/bin/run.sh"), 100) // not a .NET bin
	res, err := scanner(home).Scan(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := byRule(res)["dotnet-build"]
	if len(got) != 1 || len(got[0].Paths) != 2 {
		t.Fatalf("want bin and obj as one item, got %+v", got)
	}
}

func TestOllamaModelsAreListed(t *testing.T) {
	home := t.TempDir()
	models := filepath.Join(home, ".ollama/models")
	man := func(rel, body string) {
		write(t, filepath.Join(models, "manifests", rel), 0)
		os.WriteFile(filepath.Join(models, "manifests", rel), []byte(body), 0o644)
	}
	man("registry.ollama.ai/library/llama3.1/8b", `{"config":{"digest":"sha256:c","size":10},"layers":[{"digest":"sha256:base","size":4000},{"digest":"sha256:t1","size":20}]}`)
	man("registry.ollama.ai/sam/coder/latest", `{"config":{"digest":"sha256:c2","size":10},"layers":[{"digest":"sha256:base","size":4000},{"digest":"sha256:t2","size":30}]}`)
	man("registry.ollama.ai/library/tiny/latest", `{"config":{"digest":"sha256:c3","size":5},"layers":[{"digest":"sha256:x","size":100}]}`)
	write(t, filepath.Join(models, "blobs/sha256-base"), 4000)

	res, err := scanner(home).Scan(context.Background(), Options{SkipProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	items := byRule(res)["ollama-models"]
	if len(items) != 1 {
		t.Fatalf("want one Ollama item, got %d", len(items))
	}
	d := items[0].Details
	if len(d) != 3 {
		t.Fatalf("want 3 models, got %+v", d)
	}
	names := map[string]Detail{}
	for _, x := range d {
		names[x.Name] = x
	}
	if x, ok := names["llama3.1:8b"]; !ok || !x.Shared || x.Bytes != 4030 || x.Command != "ollama rm llama3.1:8b" {
		t.Errorf("llama3.1:8b = %+v", x)
	}
	if x, ok := names["sam/coder:latest"]; !ok || !x.Shared {
		t.Errorf("sam/coder:latest = %+v", x)
	}
	if x := names["tiny:latest"]; x.Shared {
		t.Error("tiny shares nothing")
	}
}

func TestParseDockerDF(t *testing.T) {
	out := []byte(`{"Active":"2","Reclaimable":"9.8GB (79%)","Size":"12.4GB","TotalCount":"14","Type":"Images"}
{"Active":"1","Reclaimable":"0B (0%)","Size":"2.1kB","TotalCount":"3","Type":"Containers"}
{"Active":"1","Reclaimable":"512MB (12%)","Size":"4.3GB","TotalCount":"5","Type":"Local Volumes"}
not json
{"Active":"0","Reclaimable":"3.2GB","Size":"3.2GB","TotalCount":"40","Type":"Build Cache"}`)
	d := parseDockerDF(out)
	if len(d) != 4 {
		t.Fatalf("want 4 rows, got %+v", d)
	}
	if d[0].Bytes != 12_400_000_000 || d[0].Command != "docker image prune -a" || d[0].Name != "Images" || d[0].Reclaimable != 9_800_000_000 {
		t.Errorf("images row = %+v", d[0])
	}
	if d[1].Name != "Containers" || d[1].Bytes != 2100 {
		t.Errorf("containers row = %+v", d[1])
	}
	if d[3].Bytes != 3_200_000_000 {
		t.Errorf("build cache row = %+v", d[3])
	}
}

func TestEntryLabel(t *testing.T) {
	for in, want := range map[string]string{
		"models--meta-llama--Llama-3.1-8B":   "meta-llama/Llama-3.1-8B",
		"datasets--squad":                    "squad",
		"MyApp-bxyzqwertyuiopasdfghjklzxcvb": "MyApp",
		"chromium-1243":                      "chromium-1243",
		"IntelliJIdea2024.1":                 "IntelliJIdea2024.1",
	} {
		if got := entryLabel(in); got != want {
			t.Errorf("%s: got %q want %q", in, got, want)
		}
	}
}

func TestVaultsOnOtherDrivesAreNotRescanned(t *testing.T) {
	home := t.TempDir()
	// A cleaned project's node_modules, sitting in a vault, still has
	// nested packages with their own package.json files.
	write(t, filepath.Join(home, ".leftover-vault/20260101-000000-abc123/items/0/react/package.json"), 10)
	write(t, filepath.Join(home, ".leftover-vault/20260101-000000-abc123/items/0/react/node_modules/x.js"), 9_000)
	res, err := scanner(home).Scan(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(byRule(res)["node-modules"]); n != 0 {
		t.Fatalf("items inside a vault were reported again: %d", n)
	}
}

// Editors built on VS Code ship each extension with its own node_modules.
// Removing them breaks the extension and npm install doesn't bring them
// back, so they must never show up as a project's leftovers.
func TestEditorExtensionsAreNotProjects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Linux host layout")
	}
	home := t.TempDir()
	ext := func(base string) {
		write(t, filepath.Join(home, base, "package.json"), 10)
		write(t, filepath.Join(home, base, "node_modules/dep/index.js"), 5_000)
	}
	ext(".qoder/extensions/acme.tool-1.0.0")        // a known editor folder
	ext(".neweditor/extensions/acme.tool-1.0.0")    // an editor we've never heard of
	ext("tools/portable-editor/data/acme.tool-1.0") // anywhere, marked by its manifest
	write(t, filepath.Join(home, "tools/portable-editor/data/acme.tool-1.0/.vsixmanifest"), 10)
	ext("code/app") // a real project, for contrast

	res, err := scanner(home).Scan(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range byRule(res)["node-modules"] {
		got = append(got, it.Paths[0])
	}
	want := filepath.Join(home, "code/app/node_modules")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("node_modules found = %v, want only %s", got, want)
	}
}

// A folder that looks generated but holds committed files, like a vendor
// folder someone checked in, is shown but never offered for cleaning.
func TestCommittedFoldersAreNotCleanable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Linux host layout")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git isn't installed")
	}
	home := t.TempDir()
	write(t, filepath.Join(home, "code/php/composer.json"), 10)
	write(t, filepath.Join(home, "code/php/vendor/autoload.php"), 4_000)
	write(t, filepath.Join(home, "code/web/package.json"), 10)
	write(t, filepath.Join(home, "code/web/node_modules/a.js"), 4_000)
	run := func(dir string, args ...string) {
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	repo := filepath.Join(home, "code")
	run(repo, "init", "-q")
	run(repo, "add", "php/composer.json", "php/vendor", "web/package.json")
	run(repo, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "x")

	res, err := scanner(home).Scan(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := byRule(res)
	if v := got["composer-vendor"]; len(v) != 1 || v[0].Cleanable() || len(v[0].Notes) == 0 {
		t.Fatalf("committed vendor should be shown as not cleanable with a note, got %+v", v)
	}
	if nm := got["node-modules"]; len(nm) != 1 || !nm[0].Cleanable() {
		t.Fatalf("untracked node_modules should stay cleanable, got %+v", nm)
	}
}

func TestMoreProjectTypes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Linux host layout")
	}
	home := t.TempDir()
	// Unity: Library next to ProjectSettings, with Unity's own databases inside.
	write(t, filepath.Join(home, "games/ship/ProjectSettings/ProjectVersion.txt"), 10)
	write(t, filepath.Join(home, "games/ship/Library/ArtifactDB"), 9_000)
	// A folder called Library that isn't Unity's.
	write(t, filepath.Join(home, "books/ProjectSettings/x"), 10)
	write(t, filepath.Join(home, "books/Library/novel.epub"), 9_000)
	// Elixir.
	write(t, filepath.Join(home, "code/ex/mix.exs"), 10)
	write(t, filepath.Join(home, "code/ex/_build/dev/lib/app.beam"), 4_000)
	write(t, filepath.Join(home, "code/ex/deps/plug/mix.exs"), 4_000)
	// Zig.
	write(t, filepath.Join(home, "code/zg/build.zig"), 10)
	write(t, filepath.Join(home, "code/zg/.zig-cache/o/x"), 4_000)
	write(t, filepath.Join(home, "code/zg/zig-out/bin/app"), 4_000)
	// Test coverage, and a coverage folder that's just someone's notes.
	write(t, filepath.Join(home, "code/web/package.json"), 10)
	write(t, filepath.Join(home, "code/web/coverage/lcov.info"), 4_000)
	write(t, filepath.Join(home, "code/notes/package.json"), 10)
	write(t, filepath.Join(home, "code/notes/coverage/insurance.pdf"), 4_000)
	write(t, filepath.Join(home, "code/web/.nyc_output/processinfo/index.json"), 4_000)
	write(t, filepath.Join(home, "code/py/pyproject.toml"), 10)
	write(t, filepath.Join(home, "code/py/htmlcov/status.json"), 4_000)
	write(t, filepath.Join(home, "code/notes/htmlcov/status.json"), 4_000) // no Python project here
	// Godot is shown but never offered: it can hold export credentials.
	write(t, filepath.Join(home, "games/plat/project.godot"), 10)
	write(t, filepath.Join(home, "games/plat/.godot/imported/a.ctex"), 4_000)

	res, err := scanner(home).Scan(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := byRule(res)
	for rule, n := range map[string]int{"unity-library": 1, "elixir-build": 1, "elixir-deps": 1, "zig-cache": 2, "coverage-report": 1, "nyc-output": 1, "coverage-py-html": 1, "godot-cache": 1} {
		if len(got[rule]) != n {
			t.Errorf("%s: found %d, want %d", rule, len(got[rule]), n)
		}
	}
	if g := got["godot-cache"]; len(g) == 1 && g[0].Cleanable() {
		t.Error(".godot must not be cleanable")
	}
	for _, it := range res.Items {
		if strings.Contains(it.Paths[0], "books") || strings.Contains(it.Paths[0], "notes") {
			t.Errorf("matched something that isn't generated: %s (%s)", it.Paths[0], it.Rule)
		}
	}
}

func TestTree(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "big/a.bin"), 3<<20)
	write(t, filepath.Join(root, "big/inner/b.bin"), 2<<20)
	write(t, filepath.Join(root, "small/c.txt"), 100)
	write(t, filepath.Join(root, "movie.mp4"), 4<<20)
	n, err := NewSizer(0).Tree(context.Background(), root, TreeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Children) != 2 || n.Children[0].Name != "big" || n.Children[1].Name != "movie.mp4" {
		var names []string
		for _, c := range n.Children {
			names = append(names, c.Name)
		}
		t.Fatalf("children = %v, want big then movie.mp4 (small folded into rest)", names)
	}
	if n.Rest <= 0 || n.Files != 4 {
		t.Fatalf("rest = %d, files = %d", n.Rest, n.Files)
	}
	if n.Bytes < 9<<20 {
		t.Fatalf("total %d is too small", n.Bytes)
	}
	inner := n.Find(filepath.Join(root, "big", "inner"))
	if inner == nil || inner.Bytes < 2<<20 {
		t.Fatalf("Find(big/inner) = %+v", inner)
	}
	if n.Find(filepath.Join(root, "small")) != nil {
		t.Fatal("a folder below the threshold shouldn't be kept")
	}
	if n.Find(filepath.Dir(root)) != nil {
		t.Fatal("Find must not return anything outside the tree")
	}
}

func TestAgentScratchAndOldTempFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	tmp := filepath.Join(root, "tmp")
	os.MkdirAll(home, 0o755)
	// Claude Code's scratch folder sits in the shared temp folder, so the
	// temp rule must not list it again.
	scratch := filepath.Join(tmp, "claude-501")
	write(t, filepath.Join(scratch, "-Users-sam-app/shot.png"), 40_000)
	write(t, filepath.Join(tmp, "old-build/out.bin"), 30_000)
	write(t, filepath.Join(tmp, "fresh/out.bin"), 30_000)
	old := time.Now().Add(-30 * 24 * time.Hour)
	for _, p := range []string{filepath.Join(tmp, "old-build/out.bin"), filepath.Join(tmp, "old-build"), filepath.Join(scratch, "-Users-sam-app/shot.png")} {
		os.Chtimes(p, old, old)
	}

	s := &Scanner{Catalog: rules.MustLoad(), Host: rules.Host{OS: "linux", Home: home, UID: "501", Getenv: func(k string) string {
		if k == "TMPDIR" || k == "LEFTOVER_TMP" {
			return tmp
		}
		return ""
	}}}
	res, err := s.Scan(context.Background(), Options{SkipProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	got := byRule(res)
	if n := len(got["claude-code-scratch"]); n != 1 {
		t.Fatalf("want 1 scratch folder, got %d", n)
	}
	if it := got["claude-code-scratch"][0]; it.Verdict != rules.Review || it.Cleanable() {
		t.Errorf("agent scratch must be review-only, got %s", it.Verdict)
	}
	temp := got["unix-temp"]
	if len(temp) != 1 || filepath.Base(temp[0].Paths[0]) != "old-build" {
		t.Fatalf("want only old-build as an old temp file, got %+v", temp)
	}
	if temp[0].Cleanable() {
		t.Error("old temp files on Unix must be review-only")
	}
}
