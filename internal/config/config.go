// Package config knows where Leftover keeps its own files and holds the
// handful of settings a person can change.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// DataDir is the folder Leftover writes to: settings, the history log and
// the vault for the home drive. LEFTOVER_HOME overrides it, which tests and
// portable installs use.
func DataDir() (string, error) {
	if d := os.Getenv("LEFTOVER_HOME"); d != "" {
		return filepath.Abs(d)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Leftover"), nil
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "Leftover"), nil
		}
		return filepath.Join(home, "AppData", "Local", "Leftover"), nil
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, "leftover"), nil
		}
		return filepath.Join(home, ".local", "share", "leftover"), nil
	}
}

// Settings are the choices a person can make. Zero values mean defaults.
type Settings struct {
	// KeepDays is how long cleaned items stay restorable in the vault.
	KeepDays int `json:"keep_days"`
	// AutoEmpty empties expired vault entries when Leftover starts a scan.
	// A pointer so that "unset" can default to true.
	AutoEmpty *bool `json:"auto_empty,omitempty"`
	// Ignore lists folders Leftover never reports or enters.
	Ignore []string `json:"ignore,omitempty"`
	// Roots are where to look for projects. Empty means the home folder.
	Roots []string `json:"roots,omitempty"`
}

// DefaultKeepDays is how long the vault holds items unless told otherwise.
const DefaultKeepDays = 14

// Keep returns KeepDays with the default applied.
func (s Settings) Keep() int {
	if s.KeepDays <= 0 {
		return DefaultKeepDays
	}
	return s.KeepDays
}

// ShouldAutoEmpty returns AutoEmpty with the default applied.
func (s Settings) ShouldAutoEmpty() bool { return s.AutoEmpty == nil || *s.AutoEmpty }

func settingsPath(dir string) string { return filepath.Join(dir, "settings.json") }

// Load reads the settings file, returning defaults if there isn't one.
func Load(dir string) (Settings, error) {
	var s Settings
	raw, err := os.ReadFile(settingsPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(raw, &s)
}

// Save writes the settings file atomically.
func Save(dir string, s Settings) error {
	s.Ignore = dedupe(s.Ignore)
	s.Roots = dedupe(s.Roots)
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(settingsPath(dir), append(raw, '\n'))
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// WriteFileAtomic writes to a temporary file next to path and renames it
// into place, so a crash never leaves a half-written file behind.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
