package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

func setup(t *testing.T) (*Vault, string) {
	t.Helper()
	base := t.TempDir()
	v, err := Open(filepath.Join(base, "data"), 14)
	if err != nil {
		t.Fatal(err)
	}
	v.Home = filepath.Join(base, "home")
	os.MkdirAll(v.Home, 0o755)
	return v, base
}

func mk(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func req(paths ...string) Request {
	return Request{Rule: "node-modules", Name: "node_modules", Verdict: rules.Redownload, Bytes: 10, Paths: paths}
}

func TestMoveAndRestore(t *testing.T) {
	v, _ := setup(t)
	nm := filepath.Join(v.Home, "web", "node_modules")
	mk(t, filepath.Join(nm, "react", "index.js"), "hello")

	e, err := v.Move(req(nm))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(nm); !os.IsNotExist(err) {
		t.Fatal("folder should be gone from its original place")
	}
	list, _ := v.List()
	if len(list) != 1 || list[0].ID != e.ID || list[0].State != Held {
		t.Fatalf("vault should list the entry as held: %+v", list)
	}
	if got := e.Expires.Sub(e.Created); got != 14*24*time.Hour {
		t.Errorf("expiry window = %v", got)
	}

	if _, restored, err := v.Restore(e.ID, true); err != nil || len(restored) != 1 {
		t.Fatalf("restore: %v %v", restored, err)
	}
	b, err := os.ReadFile(filepath.Join(nm, "react", "index.js"))
	if err != nil || string(b) != "hello" {
		t.Fatal("restored content differs")
	}
	if list, _ := v.List(); len(list) != 0 {
		t.Fatal("restored entry should leave the vault")
	}
	hist, _ := v.History(0)
	if len(hist) != 2 || hist[0].Action != "restore" || hist[1].Action != "move" {
		t.Fatalf("history should record move then restore: %+v", hist)
	}
}

func TestRestoreBesideWhenRecreated(t *testing.T) {
	v, _ := setup(t)
	nm := filepath.Join(v.Home, "web", "node_modules")
	mk(t, filepath.Join(nm, "old.js"), "old")
	e, err := v.Move(req(nm))
	if err != nil {
		t.Fatal(err)
	}
	mk(t, filepath.Join(nm, "new.js"), "new") // npm install ran again

	if _, _, err := v.Restore(e.ID, true); err == nil {
		t.Fatal("strict restore must refuse to overwrite")
	}
	_, restored, err := v.Restore(e.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || !strings.HasSuffix(restored[0], "node_modules (restored)") {
		t.Fatalf("should restore beside the new folder, got %v", restored)
	}
	if _, err := os.Stat(filepath.Join(nm, "new.js")); err != nil {
		t.Fatal("the new folder must be left alone")
	}
}

func TestMoveIsAllOrNothing(t *testing.T) {
	v, _ := setup(t)
	a := filepath.Join(v.Home, "p", "a", "__pycache__")
	b := filepath.Join(v.Home, "p", "b", "__pycache__")
	mk(t, filepath.Join(a, "x.pyc"), "x")
	mk(t, filepath.Join(b, "y.pyc"), "y")
	// Make the second one fail to move by locking its parent.
	parent := filepath.Dir(b)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0o755)
	if os.Getuid() == 0 {
		t.Skip("root ignores permissions")
	}
	if _, err := v.Move(req(a, b)); err == nil {
		t.Fatal("expected the move to fail")
	}
	if _, err := os.Stat(filepath.Join(a, "x.pyc")); err != nil {
		t.Fatal("the first folder must be put back after a failed move")
	}
	if list, _ := v.List(); len(list) != 0 {
		t.Fatal("a failed move must not leave an entry behind")
	}
}

func TestGuards(t *testing.T) {
	v, base := setup(t)
	mk(t, filepath.Join(v.Home, "x"), "x")
	link := filepath.Join(v.Home, "link")
	os.Symlink(filepath.Join(v.Home, "x"), link)

	cases := map[string]Request{
		"home":           req(v.Home),
		"above home":     req(base),
		"data dir":       req(v.Dir),
		"inside vault":   req(filepath.Join(v.Dir, "vault")),
		"link":           req(link),
		"relative":       req("web/node_modules"),
		"missing":        req(filepath.Join(v.Home, "nope")),
		"drive root":     req(string(filepath.Separator)),
		"not cleanable":  {Verdict: rules.Keep, Paths: []string{filepath.Join(v.Home, "x")}},
		"review only":    {Verdict: rules.Review, Paths: []string{filepath.Join(v.Home, "x")}},
		"nothing at all": {Verdict: rules.Rebuild},
	}
	for name, r := range cases {
		if _, err := v.Move(r); err == nil {
			t.Errorf("%s: move should have been refused", name)
		}
	}
	if _, err := os.Stat(filepath.Join(v.Home, "x")); err != nil {
		t.Fatal("guarded file was touched")
	}
}

func TestEmptyExpired(t *testing.T) {
	v, _ := setup(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	v.Now = func() time.Time { return now }

	old := filepath.Join(v.Home, "old", "target")
	fresh := filepath.Join(v.Home, "fresh", "target")
	mk(t, filepath.Join(old, "bin"), "x")
	mk(t, filepath.Join(fresh, "bin"), "y")
	eOld, err := v.Move(req(old))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * 24 * time.Hour)
	eFresh, err := v.Move(req(fresh))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * 24 * time.Hour) // old is 15 days in, fresh 5

	gone, err := v.EmptyExpired()
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 1 || gone[0].ID != eOld.ID {
		t.Fatalf("only the expired entry should go, got %+v", gone)
	}
	list, _ := v.List()
	if len(list) != 1 || list[0].ID != eFresh.ID {
		t.Fatalf("fresh entry should remain: %+v", list)
	}
}

func TestEmptyHandlesReadOnlyTrees(t *testing.T) {
	v, _ := setup(t)
	mod := filepath.Join(v.Home, "go", "pkg", "mod")
	mk(t, filepath.Join(mod, "example.com", "m@v1", "go.mod"), "module m")
	os.Chmod(filepath.Join(mod, "example.com", "m@v1"), 0o555)
	e, err := v.Move(req(mod))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Empty(e.ID); err != nil {
		t.Fatalf("emptying a read-only tree should still work: %v", err)
	}
}

func TestBadIDsRejected(t *testing.T) {
	v, _ := setup(t)
	for _, id := range []string{"", "..", "../x", `a\b`, "a/b"} {
		if _, err := v.Empty(id); err == nil {
			t.Errorf("id %q should be rejected", id)
		}
	}
}

func TestCrashMidMoveStillRestorable(t *testing.T) {
	v, _ := setup(t)
	nm := filepath.Join(v.Home, "web", "node_modules")
	mk(t, filepath.Join(nm, "a.js"), "a")
	e, err := v.Move(req(nm))
	if err != nil {
		t.Fatal(err)
	}
	// Pretend Leftover stopped before marking the entry held.
	e.State = Moving
	if err := writeManifest(e); err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.Restore(e.ID, true); err != nil {
		t.Fatalf("a half-finished entry must still restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nm, "a.js")); err != nil {
		t.Fatal("file not back")
	}
}
