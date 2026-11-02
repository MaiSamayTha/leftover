// Package vault is where cleaned items go instead of being deleted.
//
// Moving into the vault is a rename on the same drive: instant, and it can be
// undone exactly. Items stay restorable for a set number of days. Only Empty
// and EmptyExpired remove files, and only files that are inside a vault.
package vault

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MaiSamayTha/leftover/internal/config"
	"github.com/MaiSamayTha/leftover/internal/rules"
)

// State of a vault entry.
const (
	// Moving means the entry was being filled when Leftover stopped. Its
	// items can still be restored.
	Moving = "moving"
	// Held means everything was moved in and is waiting out its time.
	Held = "held"
)

// Entry is one cleaning action that can be undone as a unit.
type Entry struct {
	ID      string        `json:"id"`
	State   string        `json:"state"`
	Created time.Time     `json:"created"`
	Expires time.Time     `json:"expires"`
	Item    string        `json:"item_id,omitempty"`
	Rule    string        `json:"rule"`
	Name    string        `json:"name"`
	Tool    string        `json:"tool"`
	Verdict rules.Verdict `json:"verdict"`
	Bytes   int64         `json:"bytes"`
	Project string        `json:"project,omitempty"`
	// Paths are the original locations, in the order they were moved.
	Paths []string `json:"paths"`

	dir string // the entry's folder inside a vault
}

// Expired reports whether the entry's restore window has passed.
func (e Entry) Expired(now time.Time) bool { return !now.Before(e.Expires) }

// Request describes what to move.
type Request struct {
	Item    string
	Rule    string
	Name    string
	Tool    string
	Verdict rules.Verdict
	Bytes   int64
	Project string
	Paths   []string
}

// Vault manages every vault folder Leftover has made.
type Vault struct {
	// Dir is Leftover's data folder. The vault for its drive lives inside.
	Dir      string
	KeepDays int
	// Home is protected from ever being moved, along with every folder above it.
	Home string
	Now  func() time.Time

	mu sync.Mutex
}

// Open returns a vault rooted in Leftover's data folder.
func Open(dir string, keepDays int) (*Vault, error) {
	if err := os.MkdirAll(filepath.Join(dir, "vault"), 0o700); err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	if keepDays <= 0 {
		keepDays = config.DefaultKeepDays
	}
	return &Vault{Dir: dir, KeepDays: keepDays, Home: home, Now: time.Now}, nil
}

func (v *Vault) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// ErrNotCleanable is returned for items Leftover must never move.
var ErrNotCleanable = errors.New("this item is marked as your data or as unverified, so Leftover won't move it")

// ErrOtherDrive is returned when no vault can be made on an item's drive.
var ErrOtherDrive = errors.New("can't make a vault on this drive, and Leftover never copies between drives")

// Move puts every path of a request into one new vault entry. If any path
// can't be moved, the ones already moved are put back and nothing changes.
func (v *Vault) Move(req Request) (Entry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !req.Verdict.Cleanable() {
		return Entry{}, ErrNotCleanable
	}
	if len(req.Paths) == 0 {
		return Entry{}, errors.New("nothing to move")
	}
	var root string
	for i, p := range req.Paths {
		if err := v.guard(p); err != nil {
			return Entry{}, err
		}
		r, err := v.rootFor(p)
		if err != nil {
			return Entry{}, err
		}
		if i == 0 {
			root = r
		} else if r != root {
			return Entry{}, errors.New("all folders of one item must be on the same drive")
		}
	}

	now := v.now()
	e := Entry{
		ID: newID(now), State: Moving, Created: now,
		Expires: now.Add(time.Duration(v.KeepDays) * 24 * time.Hour),
		Item:    req.Item, Rule: req.Rule, Name: req.Name, Tool: req.Tool, Verdict: req.Verdict,
		Bytes: req.Bytes, Project: req.Project, Paths: append([]string(nil), req.Paths...),
	}
	e.dir = filepath.Join(root, e.ID)
	if err := os.MkdirAll(filepath.Join(e.dir, "items"), 0o700); err != nil {
		return Entry{}, err
	}
	// The manifest is written before anything moves, so even a crash
	// mid-way leaves a record of where things came from.
	if err := writeManifest(e); err != nil {
		os.RemoveAll(e.dir)
		return Entry{}, err
	}
	for i, p := range e.Paths {
		if err := os.Rename(p, e.slot(i)); err != nil {
			for j := i - 1; j >= 0; j-- {
				os.Rename(e.slot(j), e.Paths[j])
			}
			os.RemoveAll(e.dir)
			return Entry{}, fmt.Errorf("couldn't move %s: %w", p, friendly(err))
		}
	}
	e.State = Held
	if err := writeManifest(e); err != nil {
		return e, err
	}
	v.log("move", e)
	return e, nil
}

func (e Entry) slot(i int) string { return filepath.Join(e.dir, "items", fmt.Sprint(i)) }

// Restore puts an entry's folders back where they were. If something new
// already sits at an original location (say npm install ran again), the
// folder is restored beside it with " (restored)" added, unless strict is
// set, in which case Restore fails without changing anything.
func (v *Vault) Restore(id string, strict bool) (Entry, []string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, err := v.find(id)
	if err != nil {
		return Entry{}, nil, err
	}
	targets := make([]string, len(e.Paths))
	for i, p := range e.Paths {
		if _, err := os.Lstat(e.slot(i)); err != nil {
			targets[i] = "" // never moved in, or already restored
			continue
		}
		t := p
		if _, err := os.Lstat(t); err == nil {
			if strict {
				return e, nil, fmt.Errorf("%s exists again; move it away or restore beside it", p)
			}
			t = freeName(p)
		}
		targets[i] = t
	}
	var restored []string
	for i, t := range targets {
		if t == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(t), 0o755); err != nil {
			return e, restored, err
		}
		if err := os.Rename(e.slot(i), t); err != nil {
			return e, restored, fmt.Errorf("couldn't restore %s: %w", t, friendly(err))
		}
		restored = append(restored, t)
	}
	if err := v.removeEntry(e); err != nil {
		return e, restored, err
	}
	v.log("restore", e)
	return e, restored, nil
}

func freeName(p string) string {
	for i := 1; ; i++ {
		suffix := " (restored)"
		if i > 1 {
			suffix = fmt.Sprintf(" (restored %d)", i)
		}
		c := p + suffix
		if _, err := os.Lstat(c); errors.Is(err, fs.ErrNotExist) {
			return c
		}
	}
}

// Empty permanently removes one entry from the vault.
func (v *Vault) Empty(id string) (Entry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, err := v.find(id)
	if err != nil {
		return Entry{}, err
	}
	if err := v.removeEntry(e); err != nil {
		return e, err
	}
	v.log("empty", e)
	return e, nil
}

// EmptyExpired removes every entry whose restore window has passed.
func (v *Vault) EmptyExpired() ([]Entry, error) {
	entries, err := v.List()
	if err != nil {
		return nil, err
	}
	var gone []Entry
	now := v.now()
	for _, e := range entries {
		if !e.Expired(now) {
			continue
		}
		if _, err := v.Empty(e.ID); err != nil {
			return gone, err
		}
		gone = append(gone, e)
	}
	return gone, nil
}

// EmptyAll removes every entry, expired or not.
func (v *Vault) EmptyAll() ([]Entry, error) {
	entries, err := v.List()
	if err != nil {
		return nil, err
	}
	var gone []Entry
	for _, e := range entries {
		if _, err := v.Empty(e.ID); err != nil {
			return gone, err
		}
		gone = append(gone, e)
	}
	return gone, nil
}

// removeEntry deletes an entry's folder, after checking it really is one.
func (v *Vault) removeEntry(e Entry) error {
	if e.dir == "" || !v.isVaultDir(filepath.Dir(e.dir)) {
		return fmt.Errorf("refusing to remove %q: not inside a vault", e.dir)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "entry.json")); err != nil {
		return fmt.Errorf("refusing to remove %q: no manifest", e.dir)
	}
	err := os.RemoveAll(e.dir)
	if err != nil {
		makeWritable(e.dir)
		err = os.RemoveAll(e.dir)
	}
	return err
}

// List returns every entry in every vault, newest first.
func (v *Vault) List() ([]Entry, error) {
	var out []Entry
	for _, root := range v.roots() {
		dirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			e, err := readManifest(filepath.Join(root, d.Name()))
			if err != nil {
				continue
			}
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func (v *Vault) find(id string) (Entry, error) {
	if id == "" || strings.ContainsAny(id, `/\.`) {
		return Entry{}, fmt.Errorf("bad vault id %q", id)
	}
	for _, root := range v.roots() {
		if e, err := readManifest(filepath.Join(root, id)); err == nil {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("nothing in the vault with id %s", id)
}

// rootFor picks the vault for a path: the main one if the path shares its
// drive, otherwise a hidden vault at the top of the path's own drive.
func (v *Vault) rootFor(path string) (string, error) {
	main := filepath.Join(v.Dir, "vault")
	if sameVolume(path, main) {
		return main, nil
	}
	mp, err := mountPoint(path)
	if err != nil {
		return "", err
	}
	root := filepath.Join(mp, ".leftover-vault")
	if err := os.MkdirAll(root, 0o700); err != nil || !sameVolume(path, root) {
		return "", ErrOtherDrive
	}
	v.remember(root)
	return root, nil
}

// guard refuses paths that must never be moved, whatever a rule says.
func (v *Vault) guard(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%q is not a full path", path)
	}
	clean := filepath.Clean(path)
	if clean != path && clean+string(filepath.Separator) != path {
		return fmt.Errorf("%q is not a clean path", path)
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("%s is gone: %w", clean, friendly(err))
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s is a link; Leftover never moves links", clean)
	}
	if isVolumeRoot(clean) {
		return fmt.Errorf("%s is the top of a drive", clean)
	}
	if v.Home != "" && within(v.Home, clean) {
		return fmt.Errorf("%s holds your home folder", clean)
	}
	if within(clean, v.Dir) || within(v.Dir, clean) {
		return fmt.Errorf("%s overlaps Leftover's own data", clean)
	}
	for _, r := range v.roots() {
		if within(clean, r) || within(r, clean) {
			return fmt.Errorf("%s overlaps a vault", clean)
		}
	}
	return nil
}

// within reports whether child is parent or below it.
func within(child, parent string) bool {
	c, p := filepath.Clean(child), filepath.Clean(parent)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		c, p = strings.ToLower(c), strings.ToLower(p)
	}
	if c == p {
		return true
	}
	rel, err := filepath.Rel(p, c)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

func (v *Vault) isVaultDir(dir string) bool {
	for _, r := range v.roots() {
		if filepath.Clean(r) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

func (v *Vault) registryPath() string { return filepath.Join(v.Dir, "vaults.json") }

func (v *Vault) roots() []string {
	out := []string{filepath.Join(v.Dir, "vault")}
	var extra []string
	if raw, err := os.ReadFile(v.registryPath()); err == nil {
		json.Unmarshal(raw, &extra)
	}
	for _, r := range extra {
		if filepath.IsAbs(r) && filepath.Base(r) == ".leftover-vault" {
			out = append(out, r)
		}
	}
	return out
}

func (v *Vault) remember(root string) {
	all := v.roots()[1:]
	for _, r := range all {
		if r == root {
			return
		}
	}
	all = append(all, root)
	raw, _ := json.MarshalIndent(all, "", "  ")
	config.WriteFileAtomic(v.registryPath(), raw)
}

func writeManifest(e Entry) error {
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(filepath.Join(e.dir, "entry.json"), raw)
}

func readManifest(dir string) (Entry, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "entry.json"))
	if err != nil {
		return Entry{}, err
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Entry{}, err
	}
	if e.ID != filepath.Base(dir) {
		return Entry{}, fmt.Errorf("manifest id mismatch in %s", dir)
	}
	e.dir = dir
	return e, nil
}

func newID(t time.Time) string {
	b := make([]byte, 3)
	rand.Read(b)
	return t.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// friendly rewrites the errors people actually hit into plain words.
func friendly(err error) error {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return errors.New("permission denied (is an app using it, or is it owned by another user?)")
	case errors.Is(err, fs.ErrNotExist):
		return errors.New("it no longer exists")
	}
	if le, ok := err.(*os.LinkError); ok {
		return le.Err
	}
	return err
}
