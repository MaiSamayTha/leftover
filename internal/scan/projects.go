package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

// projectFinder walks your folders looking for project-level leftovers
// such as node_modules, target or .venv. It only reads folder listings and
// stats a handful of marker files; measuring happens afterwards.
type projectFinder struct {
	rules   []*rules.Rule
	skip    *pathSet // fixed locations already covered
	ignore  *pathSet
	host    rules.Host
	depth   int
	visited *atomic.Int64
	report  func(string)

	byName  map[string][]*rules.Rule
	single  []candidate
	grouped map[groupKey]*candidate
	lastRep time.Time
	rootDev uint64
}

type groupKey struct{ rule, project string }

// skipNames are never descended into while looking for projects: version
// control internals, trash, and app data folders that are handled by fixed
// location rules or simply aren't projects.
var skipNames = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".Trash": true, ".Trashes": true,
	"$RECYCLE.BIN": true, "System Volume Information": true, ".cache": true,
	".npm": true, ".cargo": true, ".rustup": true, ".m2": true, ".ollama": true,
	".docker": true, ".lmstudio": true, ".bun": true, ".nvm": true, ".pyenv": true,
	".local": true, ".vscode": true, ".cursor": true, ".android": true,
}

// homeSkips are skipped only directly under the home folder.
var homeSkips = map[string]map[string]bool{
	"darwin":  {"Library": true, "Applications": true, "Movies": true, "Music": true, "Pictures": true},
	"windows": {"AppData": true},
	"linux":   {"snap": true},
}

func (pf *projectFinder) init() {
	if pf.byName != nil {
		return
	}
	pf.byName = map[string][]*rules.Rule{}
	for _, r := range pf.rules {
		for _, d := range r.Project.Dirs {
			pf.byName[d] = append(pf.byName[d], r)
		}
	}
	pf.grouped = map[groupKey]*candidate{}
}

func (pf *projectFinder) search(ctx context.Context, root string) {
	pf.init()
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return
	}
	pf.rootDev = deviceOf(info)
	pf.walk(ctx, root, 0, "")
}

func (pf *projectFinder) walk(ctx context.Context, dir string, depth int, project string) {
	if ctx.Err() != nil {
		return
	}
	if pf.depth > 0 && depth > pf.depth {
		return
	}
	entries, err := os.ReadDir(dir)
	pf.visited.Add(1)
	if err != nil {
		return
	}
	if pf.report != nil && time.Since(pf.lastRep) > 150*time.Millisecond {
		pf.lastRep = time.Now()
		pf.report(dir)
	}

	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	// Is this folder itself a project root? Grouped items attach to the
	// nearest one.
	for _, m := range projectMarkers {
		if names[m] {
			project = dir
			break
		}
	}

	atHome := pf.host.Home != "" && norm(dir) == norm(pf.host.Home)
	for _, e := range entries {
		if !e.IsDir() || e.Type()&fs.ModeSymlink != 0 {
			continue
		}
		name := e.Name()
		child := filepath.Join(dir, name)
		if pf.ignore.has(child) || pf.skip.exact(child) {
			continue
		}
		if r := pf.match(dir, child, name, names); r != nil {
			pf.add(r, child, dir, project)
			continue // never look inside a match
		}
		if skipNames[name] || (atHome && homeSkips[pf.host.OS][name]) {
			continue
		}
		if pf.skip.has(child) {
			continue
		}
		if pf.rootDev != 0 {
			if info, err := e.Info(); err == nil && deviceOf(info) != pf.rootDev {
				continue // another file system is mounted here
			}
		}
		pf.walk(ctx, child, depth+1, project)
	}
}

// match returns the first rule a folder satisfies.
func (pf *projectFinder) match(parent, child, name string, siblings map[string]bool) *rules.Rule {
	for _, r := range pf.byName[name] {
		p := r.Project
		if len(p.Beside) > 0 {
			ok := false
			for _, b := range p.Beside {
				if siblings[b] {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if len(p.Inside) > 0 {
			ok := false
			for _, in := range p.Inside {
				if _, err := os.Lstat(filepath.Join(child, in)); err == nil {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		return r
	}
	return nil
}

func (pf *projectFinder) add(r *rules.Rule, path, parent, project string) {
	if project == "" {
		project = parent
	}
	var notes []string
	if strings.HasPrefix(r.ID, "python-venv") && venvInterpreterGone(path) {
		notes = append(notes, "The Python it was made with is gone, so this environment can't run anymore.")
	}
	if !r.Project.Group {
		pf.single = append(pf.single, candidate{rule: r, paths: []string{path}, project: parentOr(project, parent, r), notes: notes})
		return
	}
	k := groupKey{r.ID, project}
	if g, ok := pf.grouped[k]; ok {
		g.paths = append(g.paths, path)
		return
	}
	pf.grouped[k] = &candidate{rule: r, paths: []string{path}, project: project, notes: notes}
}

// parentOr picks the folder a single (ungrouped) match belongs to: the folder
// that holds it, which is where its marker file sits.
func parentOr(project, parent string, r *rules.Rule) string {
	if len(r.Project.Beside) > 0 {
		return parent
	}
	return project
}

func (pf *projectFinder) result() []candidate {
	out := append([]candidate(nil), pf.single...)
	keys := make([]groupKey, 0, len(pf.grouped))
	for k := range pf.grouped {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].project != keys[j].project {
			return keys[i].project < keys[j].project
		}
		return keys[i].rule < keys[j].rule
	})
	for _, k := range keys {
		g := pf.grouped[k]
		sort.Strings(g.paths)
		out = append(out, *g)
	}
	return out
}
