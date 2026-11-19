package ui

import (
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/MaiSamayTha/leftover/internal/config"
	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
	"github.com/MaiSamayTha/leftover/internal/update"
	"github.com/MaiSamayTha/leftover/internal/vault"
)

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("POST /api/scan", s.startScan)
	mux.HandleFunc("POST /api/scan/cancel", s.cancelScan)
	mux.HandleFunc("POST /api/clean", s.clean)
	mux.HandleFunc("POST /api/restore", s.restore)
	mux.HandleFunc("POST /api/empty", s.empty)
	mux.HandleFunc("POST /api/reveal", s.reveal)
	mux.HandleFunc("POST /api/ignore", s.ignore)
	mux.HandleFunc("POST /api/settings", s.settings)
	mux.HandleFunc("GET /api/history", s.history)
	mux.HandleFunc("POST /api/why", s.why)
	mux.HandleFunc("POST /api/docker/prune", s.dockerPrune)
	mux.HandleFunc("POST /api/explore", s.explore)
	mux.HandleFunc("POST /api/update/check", s.updateCheck)
	mux.HandleFunc("POST /api/update/apply", s.updateApply)
	mux.HandleFunc("POST /api/update/restart", s.updateRestart)
	mux.HandleFunc("POST /api/ping", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

type stateResponse struct {
	Version  string          `json:"version"`
	OS       string          `json:"os"`
	Host     string          `json:"host"`
	Home     string          `json:"home"`
	DataDir  string          `json:"data_dir"`
	Status   any             `json:"status"`
	Scan     *scan.Result    `json:"scan"`
	Vault    []vault.Entry   `json:"vault"`
	Settings config.Settings `json:"settings"`
	Now      time.Time       `json:"now"`
	// Rules has the explanation for every rule the scan's items use.
	Rules map[string]*rules.Rule `json:"rules"`
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	entries, _ := s.App.Vault.List()
	if entries == nil {
		entries = []vault.Entry{}
	}
	last := s.App.Last()
	used := map[string]*rules.Rule{}
	if last != nil {
		for _, it := range last.Items {
			if r, ok := s.App.Scanner.Catalog.Get(it.Rule); ok {
				used[it.Rule] = r
			}
		}
	}
	writeJSON(w, stateResponse{
		Version: s.Version, OS: runtime.GOOS, Host: hostname(), Home: s.App.Scanner.Host.Home,
		DataDir: s.App.Dir, Status: s.App.Status(), Scan: last, Vault: entries,
		Settings: s.App.Settings, Now: time.Now(), Rules: used,
	})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.App.Status())
}

func (s *Server) startScan(w http.ResponseWriter, r *http.Request) {
	if err := s.App.StartScan(); err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) cancelScan(w http.ResponseWriter, r *http.Request) {
	s.App.CancelScan()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clean(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := readJSON(r, &req); err != nil || len(req.IDs) == 0 {
		fail(w, http.StatusBadRequest, errors.New("pick something to clean"))
		return
	}
	if s.App.Status().Running {
		fail(w, http.StatusConflict, errors.New("wait for the scan to finish"))
		return
	}
	writeJSON(w, s.App.Clean(req.IDs))
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	e, restored, err := s.App.Vault.Restore(req.ID, false)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, map[string]any{"entry": e, "restored": restored})
}

func (s *Server) empty(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Expired bool   `json:"expired"`
		All     bool   `json:"all"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	var gone []vault.Entry
	var err error
	switch {
	case req.ID != "":
		var e vault.Entry
		e, err = s.App.Vault.Empty(req.ID)
		if err == nil {
			gone = []vault.Entry{e}
		}
	case req.Expired:
		gone, err = s.App.Vault.EmptyExpired()
	case req.All:
		gone, err = s.App.Vault.EmptyAll()
	default:
		err = errors.New("say what to empty")
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	var freed int64
	for _, e := range gone {
		freed += e.Bytes
	}
	writeJSON(w, map[string]any{"emptied": len(gone), "freed": freed})
}

// reveal opens a folder in Finder or Explorer. Only folders Leftover itself
// reported can be revealed, so the page can't be used to open arbitrary paths.
func (s *Server) reveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if !s.known(req.Path) && !s.App.Explored(req.Path) {
		fail(w, http.StatusNotFound, errors.New("not something Leftover found"))
		return
	}
	if err := reveal(req.Path); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) known(p string) bool {
	clean := filepath.Clean(p)
	if res := s.App.Last(); res != nil {
		for _, it := range res.Items {
			if it.Project != "" && filepath.Clean(it.Project) == clean {
				return true
			}
			for _, ip := range it.Paths {
				if filepath.Clean(ip) == clean {
					return true
				}
			}
		}
	}
	return false
}

func (s *Server) ignore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string `json:"path"`
		Remove bool   `json:"remove"`
	}
	if err := readJSON(r, &req); err != nil || req.Path == "" {
		fail(w, http.StatusBadRequest, errors.New("which folder?"))
		return
	}
	var err error
	if req.Remove {
		err = s.App.Unignore(req.Path)
	} else {
		err = s.App.Ignore(req.Path)
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeepDays  *int  `json:"keep_days"`
		AutoEmpty *bool `json:"auto_empty"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	st := s.App.Settings
	if req.KeepDays != nil {
		if *req.KeepDays < 1 || *req.KeepDays > 365 {
			fail(w, http.StatusBadRequest, errors.New("keep items between 1 and 365 days"))
			return
		}
		st.KeepDays = *req.KeepDays
		s.App.Vault.KeepDays = *req.KeepDays
	}
	if req.AutoEmpty != nil {
		st.AutoEmpty = req.AutoEmpty
	}
	if err := config.Save(s.App.Dir, st); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	s.App.Settings = st
	writeJSON(w, st)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	events, err := s.App.Vault.History(200)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	if events == nil {
		events = []vault.Event{}
	}
	writeJSON(w, events)
}

// why explains any folder, like 'leftover why': what it is, the verdict and
// how it comes back. It only matches the path against the rules and reads
// nothing inside the folder.
func (s *Server) why(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Path) == "" {
		fail(w, http.StatusBadRequest, errors.New("which folder?"))
		return
	}
	p := expandHome(strings.TrimSpace(req.Path), s.App.Scanner.Host.Home)
	if !filepath.IsAbs(p) {
		fail(w, http.StatusBadRequest, errors.New("use a full path, like ~/code/app/node_modules"))
		return
	}
	p = filepath.Clean(p)
	rule, matched := s.App.Explain(p)
	writeJSON(w, map[string]any{"path": p, "known": rule != nil, "matched": matched, "rule": rule})
}

// expandHome turns a leading ~ into the home folder.
func expandHome(p, home string) string {
	if home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// dockerPrune runs Docker's own cleanup for its build cache and unused
// images. Containers and volumes are never touched.
func (s *Server) dockerPrune(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Images bool `json:"images"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	steps, err := s.App.DockerPrune(r.Context(), req.Images)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, steps)
}

// explore measures a folder and lists what's inside, biggest first. It
// only reads. Leaving out the path explores the home folder.
func (s *Server) explore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Refresh bool   `json:"refresh"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	p := strings.TrimSpace(req.Path)
	if p == "" {
		p = s.App.Scanner.Host.Home
	}
	p = expandHome(p, s.App.Scanner.Host.Home)
	if !filepath.IsAbs(p) {
		fail(w, http.StatusBadRequest, errors.New("use a full path, like ~/code"))
		return
	}
	v, err := s.App.Explore(r.Context(), filepath.Clean(p), req.Refresh, nil)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, v)
}

// updateCheck asks GitHub whether there's a newer Leftover. This and
// updateApply are the only requests Leftover makes to the internet, and
// only because someone clicked the button.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	info, err := update.Check(r.Context(), s.Version)
	if err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, info)
}

// updateApply downloads, verifies and installs the latest version. The
// window then offers to restart into it.
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	info, err := update.Check(r.Context(), s.Version)
	if err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	applied, err := update.Apply(r.Context(), info)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	s.mu.Lock()
	s.pending = applied
	s.mu.Unlock()
	writeJSON(w, applied)
}

// updateRestart opens the new version and stops this one.
func (s *Server) updateRestart(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	applied := s.pending
	s.pending = nil
	s.mu.Unlock()
	if applied == nil {
		fail(w, http.StatusConflict, errors.New("there's no installed update to restart into"))
		return
	}
	if s.App.Status().Running {
		s.mu.Lock()
		s.pending = applied
		s.mu.Unlock()
		fail(w, http.StatusConflict, errors.New("a scan is running; restart when it's done"))
		return
	}
	if err := applied.Restart(); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	go func() {
		time.Sleep(500 * time.Millisecond)
		close(s.quit)
	}()
}
