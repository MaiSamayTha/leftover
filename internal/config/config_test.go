package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDataDirUsesLeftoverHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEFTOVER_HOME", dir)

	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDataDirUsesPlatformDefault(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("asserted for Unix defaults")
	}
	home := t.TempDir()
	t.Setenv("LEFTOVER_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "share", "leftover")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSettingsDefaults(t *testing.T) {
	if got := (Settings{}).Keep(); got != DefaultKeepDays {
		t.Fatalf("Keep() = %d, want %d", got, DefaultKeepDays)
	}
	if got := (Settings{KeepDays: 3}).Keep(); got != 3 {
		t.Fatalf("Keep() = %d, want 3", got)
	}
	if !(Settings{}).ShouldAutoEmpty() {
		t.Fatal("ShouldAutoEmpty() should default to true")
	}
	no := false
	if (Settings{AutoEmpty: &no}).ShouldAutoEmpty() {
		t.Fatal("ShouldAutoEmpty() should respect false")
	}
	yes := true
	if !(Settings{AutoEmpty: &yes}).ShouldAutoEmpty() {
		t.Fatal("ShouldAutoEmpty() should respect true")
	}
}

func TestLoadMissingSettingsReturnsDefaults(t *testing.T) {
	got, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Settings{}) {
		t.Fatalf("got %+v, want zero settings", got)
	}
}

func TestLoadInvalidSettingsReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("got %v, want JSON syntax error", err)
	}
}

func TestSaveDedupesSortsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	in := Settings{
		KeepDays: 7,
		Ignore:   []string{" b ", "a", "b", "", "a"},
		Roots:    []string{"/z", "/a", "/z"},
	}
	if err := Save(dir, in); err != nil {
		t.Fatal(err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{
		KeepDays: 7,
		Ignore:   []string{"a", "b"},
		Roots:    []string{"/a", "/z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWriteFileAtomicCreatesParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "one", "two", "file.txt")
	if err := WriteFileAtomic(path, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}

func TestDataDirReturnsErrorWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME handling differs on Windows")
	}
	t.Setenv("LEFTOVER_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	if _, err := DataDir(); !errors.Is(err, os.ErrNotExist) && err == nil {
		t.Fatal("expected an error without a home directory")
	}
}
