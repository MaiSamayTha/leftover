package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

// ExploreEntry is one row in a folder's disk map.
type ExploreEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
	// Rule is what Leftover recognises this as, if anything. At is the
	// folder the rule matched, which may be this entry or one above it.
	Rule *ExploreRule `json:"rule,omitempty"`
	// Clearable is how much of it the last scan found safe to clear.
	Clearable int64 `json:"clearable,omitempty"`
	// Private names the personal library this is (Photos, Contacts,
	// Calendar). Leftover doesn't open it, so its size isn't known.
	Private string `json:"private,omitempty"`
}

// ExploreRule names a rule an entry matched.
type ExploreRule struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Tool    string        `json:"tool"`
	Verdict rules.Verdict `json:"verdict"`
	At      string        `json:"at"`
}

// ExploreView is a folder's disk map.
type ExploreView struct {
	Path      string         `json:"path"`
	Parent    string         `json:"parent,omitempty"`
	Bytes     int64          `json:"bytes"`
	Files     int64          `json:"files"`
	Rest      int64          `json:"rest"`
	Unread    int64          `json:"unread,omitempty"`
	Clearable int64          `json:"clearable,omitempty"`
	Entries   []ExploreEntry `json:"entries"`
	Measured  time.Time      `json:"measured"`
	Took      time.Duration  `json:"took_ns"`
	Cached    bool           `json:"cached"`
	Rule      *ExploreRule   `json:"rule,omitempty"`
}

// How long a measured tree is reused while drilling down.
const exploreFresh = 10 * time.Minute

// Explore measures path and lists what's inside it by size. The measured
// tree is kept for a while, so opening a folder inside it is instant;
// refresh measures again. It only reads.
func (a *App) Explore(ctx context.Context, path string, refresh bool, progress func(int64, string)) (*ExploreView, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if lib := scan.Private(path); lib != "" {
		return nil, fmt.Errorf("that's your %s library; Leftover doesn't open it, so macOS never has to ask for access", lib)
	}
	a.mu.Lock()
	tree, at, took := a.tree, a.treeAt, a.treeTook
	a.mu.Unlock()

	var node *scan.Node
	cached := false
	if !refresh && tree != nil && time.Since(at) < exploreFresh {
		node = tree.Find(path)
		cached = node != nil
	}
	if node == nil {
		start := time.Now()
		t, err := scan.NewSizer(0).Tree(ctx, path, scan.TreeOptions{Progress: progress})
		if err != nil {
			return nil, err
		}
		at, took = time.Now(), time.Since(start)
		a.mu.Lock()
		a.tree, a.treeAt, a.treeTook = t, at, took
		a.mu.Unlock()
		node = t
	}

	v := &ExploreView{
		Path: node.Path, Bytes: node.Bytes, Files: node.Files, Rest: node.Rest, Unread: node.Unread,
		Measured: at, Took: took, Cached: cached, Entries: []ExploreEntry{},
	}
	if parent := filepath.Dir(node.Path); parent != node.Path {
		v.Parent = parent
	}
	v.Rule = a.ruleFor(node.Path)
	clearable := a.clearableUnder()
	v.Clearable = clearable(node.Path)
	for _, c := range node.Children {
		e := ExploreEntry{Name: c.Name, Path: c.Path, Dir: c.Dir, Bytes: c.Bytes, Files: c.Files, Private: c.Private}
		if c.Dir {
			e.Rule = a.ruleFor(c.Path)
			e.Clearable = clearable(c.Path)
		}
		v.Entries = append(v.Entries, e)
	}
	return v, nil
}

func (a *App) ruleFor(path string) *ExploreRule {
	r, at := a.Explain(path)
	if r == nil {
		return nil
	}
	if at == "" {
		at = path
	}
	return &ExploreRule{ID: r.ID, Name: r.Name, Tool: r.Tool, Verdict: r.Verdict, At: at}
}

// clearableUnder adds up, for any folder, the cleanable items of the last
// scan that sit inside it.
func (a *App) clearableUnder() func(string) int64 {
	last := a.Last()
	return func(dir string) int64 {
		if last == nil {
			return 0
		}
		var n int64
		for _, it := range last.Items {
			if !it.Cleanable() {
				continue
			}
			for _, p := range it.Paths {
				if sameOrUnder(p, dir) {
					n += it.Bytes
					break
				}
			}
		}
		return n
	}
}

// Explored reports whether path is in the last measured tree, so the
// window can offer to show it in the file manager.
func (a *App) Explored(path string) bool {
	a.mu.Lock()
	t := a.tree
	a.mu.Unlock()
	return t != nil && t.Find(path) != nil
}
