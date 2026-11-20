package scan

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

// Item is one thing Leftover found: a cache, a model, a project's
// dependencies. Grouped rules can put several folders in one item.
type Item struct {
	ID   string `json:"id"`
	Rule string `json:"rule"`
	Tool string `json:"tool"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Label names the specific thing when a rule lists entries one by one,
	// like "meta-llama/Llama-3.1-8B" for a Hugging Face model.
	Label   string        `json:"label,omitempty"`
	Group   string        `json:"group"`
	Verdict rules.Verdict `json:"verdict"`
	Paths   []string      `json:"paths"`
	// Project is the folder of the project this belongs to, if any.
	Project string `json:"project,omitempty"`
	Size
	// Idle is how long since the project (or, for caches, anything
	// inside) was last changed.
	IdleDays int `json:"idle_days"`
	// Notes are extra findings, like a virtual environment whose Python
	// is gone.
	Notes []string `json:"notes,omitempty"`
	// Details break the item into named parts, like models in a folder.
	Details []Detail `json:"details,omitempty"`
}

// Cleanable reports whether the item may be moved to the vault.
func (it *Item) Cleanable() bool { return it.Verdict.Cleanable() }

// Options controls a scan.
type Options struct {
	// Roots are searched for projects. Empty means your home folder.
	Roots []string
	// Ignore lists folders that are never reported or entered.
	Ignore []string
	// SkipProjects turns off the project search, leaving fixed locations.
	SkipProjects bool
	// MaxDepth limits how deep the project search goes below each root.
	MaxDepth int
	// Progress, if set, is called from time to time while scanning.
	Progress func(Progress)
	// Now is used for age checks; zero means time.Now.
	Now time.Time
}

// Progress is a snapshot of a running scan.
type Progress struct {
	Phase   string `json:"phase"`
	Folders int64  `json:"folders"`
	Found   int64  `json:"found"`
	Bytes   int64  `json:"bytes"`
	Current string `json:"current,omitempty"`
}

// Result is everything one scan found.
type Result struct {
	Items    []Item        `json:"items"`
	Started  time.Time     `json:"started"`
	Took     time.Duration `json:"took_ns"`
	Folders  int64         `json:"folders"`
	Roots    []string      `json:"roots"`
	Host     string        `json:"os"`
	Warnings []string      `json:"warnings,omitempty"`
	// Disk is the drive holding your home folder, when it could be read.
	Disk *Disk `json:"disk,omitempty"`
}

// Disk is the size of a drive and how much of it is free for you.
type Disk struct {
	Total int64 `json:"total"`
	Free  int64 `json:"free"`
}

// Totals adds up a result by verdict.
func (r *Result) Totals() map[rules.Verdict]int64 {
	t := map[rules.Verdict]int64{}
	for _, it := range r.Items {
		t[it.Verdict] += it.Bytes
	}
	return t
}

// Scanner ties a catalogue to a machine.
type Scanner struct {
	Catalog *rules.Catalog
	Host    rules.Host
}

// New makes a scanner for this machine with the built-in catalogue.
func New() *Scanner {
	return &Scanner{Catalog: rules.MustLoad(), Host: rules.CurrentHost()}
}

type candidate struct {
	rule    *rules.Rule
	paths   []string
	project string
	notes   []string
	// tracked counts files inside that are committed to git.
	tracked int
}

// Scan finds and measures everything. It reads; it never writes.
func (s *Scanner) Scan(ctx context.Context, opt Options) (*Result, error) {
	start := time.Now()
	now := opt.Now
	if now.IsZero() {
		now = start
	}
	roots := opt.Roots
	if len(roots) == 0 && s.Host.Home != "" {
		roots = []string{s.Host.Home}
	}
	ignore := newPathSet(opt.Ignore)

	var found atomic.Int64
	var bytes atomic.Int64
	sizer := NewSizer(0)
	report := func(phase, current string) {
		if opt.Progress != nil {
			opt.Progress(Progress{Phase: phase, Folders: sizer.Visited.Load(), Found: found.Load(), Bytes: bytes.Load(), Current: current})
		}
	}

	// 1. Fixed locations.
	report("locations", "")
	var cands []candidate
	covered := newPathSet(nil)
	// claimed holds every location some rule reports, so a rule that lists a
	// shared folder entry by entry (like /tmp) doesn't report it twice.
	claimed := map[string]bool{}
	for _, r := range s.Catalog.LocationRules() {
		for _, loc := range r.Locations(s.Host) {
			claimed[norm(loc)] = true
		}
	}
	for _, r := range s.Catalog.LocationRules() {
		for _, loc := range r.Locations(s.Host) {
			if ignore.has(loc) || ContainsHome(loc, s.Host.Home) {
				continue
			}
			info, err := os.Lstat(loc)
			if err != nil || info.Mode()&fs.ModeSymlink != 0 {
				continue
			}
			covered.add(loc)
			if !r.Each {
				cands = append(cands, candidate{rule: r, paths: []string{loc}})
				continue
			}
			entries, err := os.ReadDir(loc)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.Type()&fs.ModeSymlink != 0 || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				p := filepath.Join(loc, e.Name())
				if !ignore.has(p) && !claimed[norm(p)] {
					cands = append(cands, candidate{rule: r, paths: []string{p}})
				}
			}
		}
	}

	// 2. Projects.
	var warnings []string
	if !opt.SkipProjects {
		report("projects", "")
		pf := &projectFinder{
			rules:   s.Catalog.ProjectRules(),
			skip:    covered,
			ignore:  ignore,
			host:    s.Host,
			depth:   opt.MaxDepth,
			visited: &sizer.Visited,
			report:  func(p string) { report("projects", p) },
		}
		for _, root := range roots {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			pf.search(ctx, root)
		}
		cands = append(cands, pf.result()...)
		markTracked(ctx, cands)
	}

	// 3. Measure everything in parallel.
	report("measuring", "")
	items := make([]Item, len(cands))
	keep := make([]bool, len(cands))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0)*2)
	for i := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			c := cands[i]
			it := Item{
				Rule: c.rule.ID, Tool: c.rule.Tool, Kind: c.rule.Kind, Name: c.rule.Name,
				Group: c.rule.Group, Verdict: c.rule.Verdict, Paths: c.paths, Project: c.project,
				Notes: c.notes,
			}
			if c.tracked > 0 {
				it.Verdict = rules.Review
				it.Notes = append(it.Notes, trackedNote(c.tracked))
			}
			for _, p := range c.paths {
				sz, err := sizer.Measure(ctx, p)
				if err != nil {
					continue
				}
				it.Bytes += sz.Bytes
				it.Logical += sz.Logical
				it.Files += sz.Files
				it.Dirs += sz.Dirs
				it.Skipped += sz.Skipped
				if sz.Newest.After(it.Newest) {
					it.Newest = sz.Newest
				}
			}
			if d := detailers[c.rule.ID]; d != nil {
				it.Details = d(c.paths[0])
			}
			// Docker's data folder on Linux belongs to root, so the
			// engine's own numbers are the better measure of it.
			if DockerRules[c.rule.ID] {
				var total int64
				for _, d := range it.Details {
					total += d.Bytes
				}
				if total > it.Bytes {
					it.Bytes = total
				}
			}
			if it.Bytes == 0 && it.Files == 0 {
				return
			}
			last := it.Newest
			if c.project != "" {
				last = projectActivity(c.project, it.Newest)
			}
			if !last.IsZero() {
				it.IdleDays = int(now.Sub(last).Hours() / 24)
				if it.IdleDays < 0 {
					it.IdleDays = 0
				}
			}
			if c.rule.MinAgeDays > 0 && (it.Newest.IsZero() || now.Sub(it.Newest) < time.Duration(c.rule.MinAgeDays)*24*time.Hour) {
				return
			}
			if c.rule.Each {
				it.Label = entryLabel(filepath.Base(c.paths[0]))
			}
			it.ID = itemID(it.Rule, it.Paths)
			items[i] = it
			keep[i] = true
			found.Add(1)
			bytes.Add(it.Bytes)
			report("measuring", c.paths[0])
		}(i)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	res := &Result{Started: start, Roots: roots, Host: s.Host.OS, Warnings: warnings}
	for i, ok := range keep {
		if ok {
			res.Items = append(res.Items, items[i])
		}
	}
	sort.SliceStable(res.Items, func(a, b int) bool {
		if res.Items[a].Bytes != res.Items[b].Bytes {
			return res.Items[a].Bytes > res.Items[b].Bytes
		}
		return res.Items[a].Paths[0] < res.Items[b].Paths[0]
	})
	res.Folders = sizer.Visited.Load()
	if total, free, ok := DiskSpace(s.Host.Home); ok && total > 0 {
		res.Disk = &Disk{Total: total, Free: free}
	}
	res.Took = time.Since(start)
	report("done", "")
	return res, nil
}

// ContainsHome reports whether loc is the home folder or a folder above it,
// like a temporary folder a test's home was made in. Such a location is
// never reported, or it would hide every project inside it.
func ContainsHome(loc, home string) bool {
	if home == "" {
		return false
	}
	rel, err := filepath.Rel(norm(loc), norm(home))
	return err == nil && !startsWithParent(rel) && !filepath.IsAbs(rel)
}

func itemID(rule string, paths []string) string {
	ps := append([]string(nil), paths...)
	sort.Strings(ps)
	h := sha1.New()
	h.Write([]byte(rule))
	for _, p := range ps {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// projectMarkers are files whose presence makes a folder a project root.
var projectMarkers = []string{
	".git", "package.json", "pyproject.toml", "setup.py", "requirements.txt", "Pipfile",
	"Cargo.toml", "go.mod", "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
	"settings.gradle.kts", "Package.swift", "Podfile", "Gemfile", "composer.json", "deno.json",
	"mix.exs", "stack.yaml", "cabal.project", "build.zig", "build.sbt", "deps.edn", "angular.json",
	"project.godot", "ProjectSettings",
}

// generated names are ignored when judging when a project was last worked on.
var generated = map[string]bool{
	"node_modules": true, ".next": true, ".nuxt": true, ".output": true, ".svelte-kit": true,
	".turbo": true, ".parcel-cache": true, "target": true, "build": true, ".gradle": true,
	".build": true, ".venv": true, "venv": true, "env": true, "__pycache__": true,
	".pytest_cache": true, ".mypy_cache": true, ".ruff_cache": true, ".tox": true, ".nox": true,
	"dist": true, ".DS_Store": true, "_build": true, "deps": true, ".stack-work": true,
	"dist-newstyle": true, ".zig-cache": true, "zig-cache": true, "zig-out": true, ".cpcache": true,
	".angular": true, ".docusaurus": true, "storybook-static": true, "coverage": true,
	"htmlcov": true, ".nyc_output": true,
	"Library": true, "Temp": true, "Intermediate": true, "DerivedDataCache": true, ".godot": true,
}

// projectActivity estimates when a project was last worked on: the newest of
// its top-level files and its git index, ignoring generated folders.
func projectActivity(dir string, fallback time.Time) time.Time {
	var newest time.Time
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fallback
	}
	for _, e := range entries {
		if generated[e.Name()] {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if info, err := os.Stat(filepath.Join(dir, ".git", "index")); err == nil && info.ModTime().After(newest) {
		newest = info.ModTime()
	}
	if newest.IsZero() {
		return fallback
	}
	return newest
}

// venvInterpreterGone reads pyvenv.cfg and reports whether the Python the
// environment was made with no longer exists.
func venvInterpreterGone(venv string) bool {
	f, err := os.Open(filepath.Join(venv, "pyvenv.cfg"))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok || strings.TrimSpace(k) != "home" {
			continue
		}
		home := strings.TrimSpace(v)
		if home == "" {
			return false
		}
		_, err := os.Stat(home)
		return os.IsNotExist(err)
	}
	return false
}

// pathSet answers "is this path, or a folder above it, in the set".
type pathSet struct{ m map[string]bool }

func newPathSet(paths []string) *pathSet {
	s := &pathSet{m: map[string]bool{}}
	for _, p := range paths {
		s.add(p)
	}
	return s
}

func norm(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

func (s *pathSet) add(p string) {
	if p != "" {
		s.m[norm(p)] = true
	}
}

func (s *pathSet) has(p string) bool {
	if len(s.m) == 0 {
		return false
	}
	p = norm(p)
	for {
		if s.m[p] {
			return true
		}
		parent := filepath.Dir(p)
		if parent == p {
			return false
		}
		p = parent
	}
}

func (s *pathSet) exact(p string) bool { return s.m[norm(p)] }

// entryLabel makes a readable name from a folder name inside a cache.
func entryLabel(name string) string {
	for _, prefix := range []string{"models--", "datasets--", "spaces--"} {
		if strings.HasPrefix(name, prefix) {
			return strings.ReplaceAll(strings.TrimPrefix(name, prefix), "--", "/")
		}
	}
	// Xcode names DerivedData folders Project-<28 random letters>.
	if i := strings.LastIndexByte(name, '-'); i > 0 && len(name)-i-1 == 28 {
		return name[:i]
	}
	return name
}

func trackedNote(n int) string {
	if n == 1 {
		return "1 file in here is committed to git, so this isn't only generated output. Leftover won't move it."
	}
	return fmt.Sprintf("%d files in here are committed to git, so this isn't only generated output. Leftover won't move it.", n)
}
