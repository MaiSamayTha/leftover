// Package rules holds the catalogue of things Leftover knows about: where each
// tool keeps its caches, what they are, and whether they come back on their own.
//
// Rules live as JSON files in the catalog folder and are compiled into the
// binary, so the app never needs the network to know what it is looking at.
package rules

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"
)

// Verdict says what happens if an item goes away.
type Verdict string

const (
	// Rebuild means your own tools recreate it the next time they run
	// (build output, compiled bytecode, indexes).
	Rebuild Verdict = "rebuild"
	// Redownload means it is fetched again from the internet when needed
	// (package caches, model weights, browser binaries).
	Redownload Verdict = "redownload"
	// Review means we know what it is but can't promise it comes back.
	// Leftover shows it and explains it but never offers to clean it.
	Review Verdict = "review"
	// Keep means it is your data. Shown so you understand the space, never cleaned.
	Keep Verdict = "keep"
)

// Cleanable reports whether items with this verdict may be moved to the vault.
func (v Verdict) Cleanable() bool { return v == Rebuild || v == Redownload }

func (v Verdict) valid() bool {
	switch v {
	case Rebuild, Redownload, Review, Keep:
		return true
	}
	return false
}

// Label is the short human wording used in the CLI and the app.
func (v Verdict) Label() string {
	switch v {
	case Rebuild:
		return "Rebuilds itself"
	case Redownload:
		return "Downloads again"
	case Review:
		return "Worth a look"
	case Keep:
		return "Your data"
	}
	return string(v)
}

// EnvPath points a rule at a location taken from an environment variable,
// optionally with a sub-path joined on, e.g. HF_HOME + "hub".
type EnvPath struct {
	Var  string `json:"var"`
	Join string `json:"join,omitempty"`
}

// Project describes a folder found inside your projects rather than at a
// fixed place, like node_modules next to a package.json.
type Project struct {
	// Dir is the folder name to look for. Several names may share one rule.
	Dirs []string `json:"dirs"`
	// Beside lists files, any one of which must sit next to the folder.
	// Empty means no neighbour is required.
	Beside []string `json:"beside,omitempty"`
	// Inside lists files, any one of which must exist inside the folder.
	Inside []string `json:"inside,omitempty"`
	// Group folds every match in one project into a single item, for things
	// that are scattered in many small pieces like __pycache__.
	Group bool `json:"group,omitempty"`
}

// Rule is one kind of thing Leftover can find.
type Rule struct {
	ID      string  `json:"id"`
	Tool    string  `json:"tool"`
	Kind    string  `json:"kind"`
	Name    string  `json:"name"`
	Verdict Verdict `json:"verdict"`
	// What explains it in plain words.
	What string `json:"what"`
	// Comeback says how it returns, or why it won't.
	Comeback string `json:"comeback,omitempty"`
	// Source links to the documentation the verdict is based on.
	Source string `json:"source,omitempty"`
	// Command is the tool's own cleanup command, when it has one.
	Command string `json:"command,omitempty"`

	// Paths maps an OS key (darwin, linux, windows, unix, all) to
	// locations. See Expand for the placeholders that are understood.
	Paths map[string][]string `json:"paths,omitempty"`
	// Env lists variables that move the location. The first one that is
	// set replaces Paths entirely.
	Env []EnvPath `json:"env,omitempty"`
	// Each lists the children of a location as separate items, e.g. one
	// item per downloaded model instead of one for the whole cache.
	Each bool `json:"each,omitempty"`
	// MinAgeDays only counts entries untouched for this many days.
	MinAgeDays int `json:"min_age_days,omitempty"`

	Project *Project `json:"project,omitempty"`

	// Group is the file the rule was loaded from, used for display.
	Group string `json:"-"`
}

type file struct {
	Group string `json:"group"`
	Rules []Rule `json:"rules"`
}

//go:embed catalog/*.json
var catalogFS embed.FS

// Catalog is a validated, ordered set of rules.
type Catalog struct {
	Rules []Rule
	byID  map[string]*Rule
}

// Load returns the built-in catalogue.
func Load() (*Catalog, error) {
	sub, err := fs.Sub(catalogFS, "catalog")
	if err != nil {
		return nil, err
	}
	return LoadFS(sub)
}

// MustLoad is Load for callers that can't continue without rules.
func MustLoad() *Catalog {
	c, err := Load()
	if err != nil {
		panic(err)
	}
	return c
}

// LoadFS reads every *.json file at the root of fsys.
func LoadFS(fsys fs.FS) (*Catalog, error) {
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	c := &Catalog{byID: map[string]*Rule{}}
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var f file
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		group := f.Group
		if group == "" {
			group = strings.TrimSuffix(path.Base(name), ".json")
		}
		for _, r := range f.Rules {
			r.Group = group
			if err := r.check(); err != nil {
				return nil, fmt.Errorf("%s: rule %q: %w", name, r.ID, err)
			}
			if _, dup := c.byID[r.ID]; dup {
				return nil, fmt.Errorf("%s: rule %q is defined twice", name, r.ID)
			}
			c.Rules = append(c.Rules, r)
			c.byID[r.ID] = &c.Rules[len(c.Rules)-1]
		}
	}
	// Re-point the index now that the slice has stopped growing.
	for i := range c.Rules {
		c.byID[c.Rules[i].ID] = &c.Rules[i]
	}
	return c, nil
}

// Get looks a rule up by id.
func (c *Catalog) Get(id string) (*Rule, bool) {
	r, ok := c.byID[id]
	return r, ok
}

// ProjectRules returns rules that are found inside projects, in catalogue order.
func (c *Catalog) ProjectRules() []*Rule {
	var out []*Rule
	for i := range c.Rules {
		if c.Rules[i].Project != nil {
			out = append(out, &c.Rules[i])
		}
	}
	return out
}

// LocationRules returns rules that live at fixed places.
func (c *Catalog) LocationRules() []*Rule {
	var out []*Rule
	for i := range c.Rules {
		if c.Rules[i].Project == nil {
			out = append(out, &c.Rules[i])
		}
	}
	return out
}

var osKeys = map[string]bool{"darwin": true, "linux": true, "windows": true, "unix": true, "all": true}

func (r *Rule) check() error {
	switch {
	case r.ID == "" || strings.ContainsAny(r.ID, " /\\"):
		return fmt.Errorf("id must be a slug")
	case r.Tool == "" || r.Name == "" || r.What == "":
		return fmt.Errorf("tool, name and what are required")
	case !r.Verdict.valid():
		return fmt.Errorf("unknown verdict %q", r.Verdict)
	}
	// The one rule we never bend: nothing is offered for cleaning without
	// a source that says it comes back.
	if r.Verdict.Cleanable() {
		u, err := url.Parse(r.Source)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("verdict %q needs an https source", r.Verdict)
		}
		if r.Comeback == "" {
			return fmt.Errorf("verdict %q needs a comeback explanation", r.Verdict)
		}
	}
	hasPaths := len(r.Paths) > 0 || len(r.Env) > 0
	if r.Project != nil {
		if hasPaths {
			return fmt.Errorf("a rule is either a project rule or a location rule, not both")
		}
		if len(r.Project.Dirs) == 0 {
			return fmt.Errorf("project rules need at least one dir")
		}
		for _, d := range r.Project.Dirs {
			if d == "" || strings.ContainsAny(d, "/\\") {
				return fmt.Errorf("project dir %q must be a plain folder name", d)
			}
		}
		return nil
	}
	if !hasPaths {
		return fmt.Errorf("needs paths, env or project")
	}
	for k, ps := range r.Paths {
		if !osKeys[k] {
			return fmt.Errorf("unknown os key %q", k)
		}
		for _, p := range ps {
			if p == "" {
				return fmt.Errorf("empty path under %q", k)
			}
		}
	}
	for _, e := range r.Env {
		if e.Var == "" {
			return fmt.Errorf("env entry without a var")
		}
	}
	return nil
}
