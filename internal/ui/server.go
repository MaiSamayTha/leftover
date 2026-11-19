// Package ui serves Leftover's desktop window. The window is a web page, but
// it is only ever served to this machine: the server listens on 127.0.0.1,
// every API call must carry a random per-launch token, and requests with a
// foreign Host or Origin header are refused, which shuts out other websites
// and DNS rebinding tricks.
package ui

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/update"
)

//go:embed web
var webFS embed.FS

// Server is one running window's backend.
type Server struct {
	App     *app.App
	Version string
	token   string
	addr    string
	ln      net.Listener

	mu       sync.Mutex
	lastSeen time.Time
	// pending is an installed update waiting to be restarted into.
	pending *update.Applied
	// quit ends Serve, after a restart into a new version.
	quit chan struct{}
}

// New prepares a server bound to a free port on 127.0.0.1.
func New(a *app.App, version string) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		ln.Close()
		return nil, err
	}
	return &Server{App: a, Version: version, token: hex.EncodeToString(b), addr: ln.Addr().String(), ln: ln, lastSeen: time.Now(), quit: make(chan struct{})}, nil
}

// URL is the address the window opens. The token travels in the fragment,
// which browsers never send to servers or put in logs.
func (s *Server) URL() string { return fmt.Sprintf("http://%s/#%s", s.addr, s.token) }

// Serve handles requests until the window has been gone for idle, then
// returns. Zero idle serves forever.
func (s *Server) Serve(idle time.Duration) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(s.ln) }()
	tick := 5 * time.Second
	if idle <= 0 {
		tick = time.Hour
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case err := <-errc:
			return err
		case <-s.quit:
			srv.Close()
			return nil
		case <-t.C:
			if idle <= 0 {
				continue
			}
			s.mu.Lock()
			gone := time.Since(s.lastSeen) > idle
			s.mu.Unlock()
			if gone && !s.App.Status().Running {
				srv.Close()
				return nil
			}
		}
	}
}

func (s *Server) seen() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}

// Handler returns the full HTTP handler with every check applied.
func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	mux := http.NewServeMux()
	mux.Handle("GET /", files)
	s.routes(mux)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only our own address may be used to reach us.
		if r.Host != s.addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+s.addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			got := r.Header.Get("X-Leftover-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			s.seen()
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return errors.New("empty request")
	}
	return json.Unmarshal(raw, v)
}

// hostname is shown in the window's title bar area.
func hostname() string {
	h, _ := os.Hostname()
	return h
}
