// Package app is the layer the command line and the desktop window share:
// it runs scans, remembers the last result, and turns choices into vault
// moves with every safety check applied.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/MaiSamayTha/leftover/internal/config"
	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
	"github.com/MaiSamayTha/leftover/internal/vault"
)

// App holds everything a session of Leftover needs.
type App struct {
	Dir      string
	Settings config.Settings
	Scanner  *scan.Scanner
	Vault    *vault.Vault

	mu       sync.Mutex
	last     *scan.Result
	tree     *scan.Node // the last disk map Explore measured
	treeAt   time.Time
	treeTook time.Duration
	running  bool
	progress scan.Progress
	cancel   context.CancelFunc
	scanErr  string
}

// Open loads settings and the vault from Leftover's data folder.
func Open() (*App, error) {
	dir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	return OpenAt(dir, scan.New())
}

// OpenAt is Open with an explicit data folder and scanner, for tests.
func OpenAt(dir string, s *scan.Scanner) (*App, error) {
	st, err := config.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	v, err := vault.Open(dir, st.Keep())
	if err != nil {
		return nil, err
	}
	if s.Host.Home != "" {
		v.Home = s.Host.Home
	}
	a := &App{Dir: dir, Settings: st, Scanner: s, Vault: v}
	a.last, _ = a.loadLast()
	return a, nil
}

func (a *App) lastPath() string { return filepath.Join(a.Dir, "last-scan.json") }

func (a *App) loadLast() (*scan.Result, error) {
	raw, err := os.ReadFile(a.lastPath())
	if err != nil {
		return nil, err
	}
	var r scan.Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Last returns the most recent scan result, or nil.
func (a *App) Last() *scan.Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last
}

// Status describes what the app is doing right now.
type Status struct {
	Running  bool          `json:"running"`
	Progress scan.Progress `json:"progress"`
	Error    string        `json:"error,omitempty"`
}

// Status reports whether a scan is running and how far along it is.
func (a *App) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Status{Running: a.running, Progress: a.progress, Error: a.scanErr}
}

// ErrBusy is returned when a scan is already running.
var ErrBusy = errors.New("a scan is already running")

// Scan runs a full scan and saves the result. Expired vault entries are
// emptied first when the setting allows it.
func (a *App) Scan(ctx context.Context, onProgress func(scan.Progress)) (*scan.Result, []vault.Entry, error) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return nil, nil, ErrBusy
	}
	ctx, cancel := context.WithCancel(ctx)
	a.running, a.cancel, a.scanErr = true, cancel, ""
	a.progress = scan.Progress{Phase: "starting"}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.running, a.cancel = false, nil
		a.mu.Unlock()
		cancel()
	}()

	var emptied []vault.Entry
	if a.Settings.ShouldAutoEmpty() {
		emptied, _ = a.Vault.EmptyExpired()
	}
	opt := scan.Options{
		Roots:  a.Settings.Roots,
		Ignore: a.ignoreList(),
		Progress: func(p scan.Progress) {
			a.mu.Lock()
			a.progress = p
			a.mu.Unlock()
			if onProgress != nil {
				onProgress(p)
			}
		},
	}
	res, err := a.Scanner.Scan(ctx, opt)
	if err != nil {
		a.mu.Lock()
		a.scanErr = err.Error()
		a.mu.Unlock()
		return nil, emptied, err
	}
	a.noteBusy(ctx, res)
	a.mu.Lock()
	a.last = res
	a.mu.Unlock()
	if raw, err := json.Marshal(res); err == nil {
		config.WriteFileAtomic(a.lastPath(), raw)
	}
	return res, emptied, nil
}

// StartScan runs Scan in the background, for the desktop window.
func (a *App) StartScan() error {
	a.mu.Lock()
	busy := a.running
	a.mu.Unlock()
	if busy {
		return ErrBusy
	}
	go a.Scan(context.Background(), nil)
	// Give the goroutine a moment to mark itself running so an immediate
	// status poll doesn't report idle.
	for i := 0; i < 50; i++ {
		if a.Status().Running {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// CancelScan stops a running scan.
func (a *App) CancelScan() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// ignoreList is the user's ignore list plus Leftover's own data folder.
func (a *App) ignoreList() []string {
	return append(append([]string(nil), a.Settings.Ignore...), a.Dir)
}

// Find returns an item from the last scan by id.
func (a *App) Find(id string) (scan.Item, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil {
		return scan.Item{}, false
	}
	for _, it := range a.last.Items {
		if it.ID == id {
			return it, true
		}
	}
	return scan.Item{}, false
}

// Outcome is the result of cleaning one item.
type Outcome struct {
	Item  string       `json:"item"`
	Name  string       `json:"name"`
	Bytes int64        `json:"bytes"`
	Entry *vault.Entry `json:"entry,omitempty"`
	Error string       `json:"error,omitempty"`
}

// Clean moves the given items from the last scan into the vault. Each item
// is checked again before it moves: it must still be cleanable, and its
// folders must still exist and not have turned into links.
func (a *App) Clean(ids []string) []Outcome {
	var out []Outcome
	bc := &busyChecker{ctx: context.Background()}
	batch := strconv.FormatInt(time.Now().UnixNano(), 36)
	for _, id := range ids {
		it, ok := a.Find(id)
		if !ok {
			out = append(out, Outcome{Item: id, Error: "not in the last scan; scan again"})
			continue
		}
		o := Outcome{Item: id, Name: it.Name, Bytes: it.Bytes}
		r, ok := a.Scanner.Catalog.Get(it.Rule)
		if !ok || !r.Verdict.Cleanable() || !it.Cleanable() {
			o.Error = vault.ErrNotCleanable.Error()
			out = append(out, o)
			continue
		}
		if bz, busy := bc.check(r); busy {
			o.Error = BusyMessage(bz)
			out = append(out, o)
			continue
		}
		e, err := a.Vault.Move(vault.Request{
			Item: it.ID, Rule: it.Rule, Name: it.Name, Tool: it.Tool, Verdict: r.Verdict, Batch: batch,
			Bytes: it.Bytes, Project: it.Project, Paths: it.Paths,
		})
		if err != nil {
			o.Error = err.Error()
		} else {
			o.Entry = &e
			a.dropFromLast(id)
		}
		out = append(out, o)
	}
	a.saveLast()
	return out
}

// Busy reports, for each item, a message when a program that uses its
// files is running right now, so it can be left out before anyone is asked.
func (a *App) Busy(items []scan.Item) map[string]string {
	bc := &busyChecker{ctx: context.Background()}
	out := map[string]string{}
	for _, it := range items {
		if r, ok := a.Scanner.Catalog.Get(it.Rule); ok {
			if bz, busy := bc.check(r); busy {
				out[it.ID] = BusyMessage(bz)
			}
		}
	}
	return out
}

// noteBusy adds a note to items Leftover only lists, like an agent's
// scratch folder, when the program that uses them is running right now.
func (a *App) noteBusy(ctx context.Context, res *scan.Result) {
	bc := &busyChecker{ctx: ctx}
	for i := range res.Items {
		it := &res.Items[i]
		if it.Verdict != rules.Review {
			continue
		}
		if r, ok := a.Scanner.Catalog.Get(it.Rule); ok {
			if bz, busy := bc.check(r); busy {
				it.Notes = append(it.Notes, bz.Name+" is running, so a task may still be using this.")
			}
		}
	}
}

func (a *App) dropFromLast(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil {
		return
	}
	items := a.last.Items[:0]
	for _, it := range a.last.Items {
		if it.ID != id {
			items = append(items, it)
		}
	}
	a.last.Items = items
}

func (a *App) saveLast() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil {
		return
	}
	if raw, err := json.Marshal(a.last); err == nil {
		config.WriteFileAtomic(a.lastPath(), raw)
	}
}

// Ignore adds a folder to the ignore list and drops matching items from the
// last scan.
func (a *App) Ignore(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	a.Settings.Ignore = append(a.Settings.Ignore, abs)
	if err := config.Save(a.Dir, a.Settings); err != nil {
		return err
	}
	a.mu.Lock()
	if a.last != nil {
		items := a.last.Items[:0]
		for _, it := range a.last.Items {
			if !sameOrUnder(it.Paths[0], abs) {
				items = append(items, it)
			}
		}
		a.last.Items = items
	}
	a.mu.Unlock()
	a.saveLast()
	return nil
}

// Unignore removes a folder from the ignore list.
func (a *App) Unignore(path string) error {
	abs, _ := filepath.Abs(path)
	var keep []string
	for _, p := range a.Settings.Ignore {
		if p != path && p != abs {
			keep = append(keep, p)
		}
	}
	a.Settings.Ignore = keep
	return config.Save(a.Dir, a.Settings)
}

// Explain finds what Leftover knows about any path, even one a scan didn't
// report. It returns nil when no rule matches.
func (a *App) Explain(path string) (*rules.Rule, string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, ""
	}
	cat := a.Scanner.Catalog
	h := a.Scanner.Host
	for i := range cat.Rules {
		r := &cat.Rules[i]
		for _, loc := range r.Locations(h) {
			if sameOrUnder(abs, loc) {
				return r, loc
			}
		}
	}
	if scan.InEditorExtension(abs) {
		if r, ok := cat.Get("editor-extensions"); ok {
			return r, ""
		}
	}
	// Walk up looking for a project folder name that matches a rule.
	for p := abs; ; {
		name := filepath.Base(p)
		parent := filepath.Dir(p)
		for _, r := range cat.ProjectRules() {
			for _, d := range r.Project.Dirs {
				if d == name && projectMatches(r, parent, p) {
					return r, p
				}
			}
		}
		if parent == p {
			break
		}
		p = parent
	}
	return nil, ""
}

func sameOrUnder(p, dir string) bool {
	if p == dir {
		return true
	}
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !startsWithDotDot(rel)
}

func startsWithDotDot(rel string) bool {
	return len(rel) >= 3 && rel[:2] == ".." && (rel[2] == '/' || rel[2] == '\\')
}

func projectMatches(r *rules.Rule, parent, dir string) bool {
	siblings := map[string]bool{}
	if entries, err := os.ReadDir(parent); err == nil {
		for _, e := range entries {
			siblings[e.Name()] = true
		}
	}
	if !r.Project.MatchesBeside(siblings) {
		return false
	}
	if len(r.Project.Inside) == 0 {
		return true
	}
	for _, in := range r.Project.Inside {
		if _, err := os.Lstat(filepath.Join(dir, in)); err == nil {
			return true
		}
	}
	return false
}
