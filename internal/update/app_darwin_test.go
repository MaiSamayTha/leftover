package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeApp builds a tiny ad-hoc signed Leftover.app whose executable prints
// what.
func makeApp(t *testing.T, dir, what string) string {
	t.Helper()
	app := filepath.Join(dir, "Leftover.app")
	exe := filepath.Join(app, "Contents", "MacOS", "Leftover")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho "+what+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleExecutable</key><string>Leftover</string><key>CFBundleIdentifier</key><string>test.leftover</string></dict></plist>`), 0o644)
	if out, err := exec.Command("codesign", "--force", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Skipf("codesign unavailable: %s", out)
	}
	return app
}

func TestReplaceApp(t *testing.T) {
	apps := t.TempDir()
	installed := makeApp(t, apps, "old")
	build := t.TempDir()
	fresh := makeApp(t, build, "new")
	zipPath := filepath.Join(build, "Leftover-macos.zip")
	if out, err := exec.Command("ditto", "-c", "-k", "--keepParent", fresh, zipPath).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %s", out)
	}
	exe := filepath.Join(installed, "Contents", "MacOS", "Leftover")
	got, err := replaceApp(zipPath, exe)
	if err != nil {
		t.Fatal(err)
	}
	if got != installed {
		t.Fatalf("replaced %s, want %s", got, installed)
	}
	out, err := exec.Command(exe).Output()
	if err != nil || string(out) != "new\n" {
		t.Fatalf("installed app runs %q, %v", out, err)
	}
	entries, _ := os.ReadDir(apps)
	if len(entries) != 1 {
		t.Fatalf("leftovers next to the app after updating: %v", entries)
	}
}
