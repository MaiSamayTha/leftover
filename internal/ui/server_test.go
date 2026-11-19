package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	home := t.TempDir()
	s := &scan.Scanner{Catalog: rules.MustLoad(), Host: rules.Host{OS: "linux", Home: home, Getenv: func(string) string { return "" }}}
	a, err := app.OpenAt(filepath.Join(home, "data"), s)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(a, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.ln.Close() })
	return srv
}

func call(srv *Server, method, path, body string, mod func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = srv.addr
	r.Header.Set("X-Leftover-Token", srv.token)
	if mod != nil {
		mod(r)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	return w
}

func TestTokenRequired(t *testing.T) {
	srv := testServer(t)
	if w := call(srv, "GET", "/api/state", "", nil); w.Code != 200 {
		t.Fatalf("with token: %d %s", w.Code, w.Body)
	}
	for name, mod := range map[string]func(*http.Request){
		"no token":       func(r *http.Request) { r.Header.Del("X-Leftover-Token") },
		"wrong token":    func(r *http.Request) { r.Header.Set("X-Leftover-Token", "nope") },
		"foreign host":   func(r *http.Request) { r.Host = "evil.example:80" },
		"rebinding":      func(r *http.Request) { r.Host = "localhost" + srv.addr[strings.LastIndex(srv.addr, ":"):] },
		"foreign origin": func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
	} {
		if w := call(srv, "GET", "/api/state", "", mod); w.Code != http.StatusForbidden {
			t.Errorf("%s: want 403, got %d", name, w.Code)
		}
	}
}

func TestPageNeedsNoTokenButKeepsHeaders(t *testing.T) {
	srv := testServer(t)
	w := call(srv, "GET", "/", "", func(r *http.Request) { r.Header.Del("X-Leftover-Token") })
	if w.Code != 200 {
		t.Fatalf("page: %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Error("missing CSP")
	}
	if strings.Contains(srv.URL(), "?") || !strings.Contains(srv.URL(), "#"+srv.token) {
		t.Error("token must travel in the fragment, never the query")
	}
}

func TestRevealOnlyKnownPaths(t *testing.T) {
	srv := testServer(t)
	if w := call(srv, "POST", "/api/reveal", `{"path":"/etc"}`, nil); w.Code != http.StatusNotFound {
		t.Fatalf("revealing an unknown path should be refused, got %d", w.Code)
	}
}

func TestCleanNeedsIDs(t *testing.T) {
	srv := testServer(t)
	if w := call(srv, "POST", "/api/clean", `{"ids":[]}`, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("got %d", w.Code)
	}
}

func TestSettingsValidated(t *testing.T) {
	srv := testServer(t)
	if w := call(srv, "POST", "/api/settings", `{"keep_days":0}`, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("0 days should be refused, got %d", w.Code)
	}
	if w := call(srv, "POST", "/api/settings", `{"keep_days":30}`, nil); w.Code != 200 {
		t.Fatalf("30 days: %d %s", w.Code, w.Body)
	}
	if srv.App.Vault.KeepDays != 30 {
		t.Error("vault should pick up the new window")
	}
}

func TestWhyExplainsAnyFolder(t *testing.T) {
	srv := testServer(t)
	home := srv.App.Scanner.Host.Home
	web := filepath.Join(home, "code", "web")
	if err := os.MkdirAll(filepath.Join(web, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(web, "package.json"), []byte("{}"), 0o644)

	w := call(srv, "POST", "/api/why", `{"path":"~/code/web/node_modules"}`, nil)
	var got struct {
		Path  string `json:"path"`
		Known bool   `json:"known"`
		Rule  struct {
			ID string `json:"id"`
		} `json:"rule"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !got.Known || got.Rule.ID != "node-modules" || got.Path != filepath.Join(web, "node_modules") {
		t.Fatalf("~/code/web/node_modules should be explained as node_modules, got %+v", got)
	}

	w = call(srv, "POST", "/api/why", `{"path":"`+filepath.ToSlash(filepath.Join(home, "notes"))+`"}`, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"known":false`) {
		t.Fatalf("an unknown folder should be reported as unknown: %d %s", w.Code, w.Body)
	}

	for _, body := range []string{`{}`, `{"path":"  "}`, `{"path":"relative/dir"}`} {
		if w := call(srv, "POST", "/api/why", body, nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", body, w.Code)
		}
	}
	if w := call(srv, "POST", "/api/why", `{"path":"~/.cache/pip"}`, func(r *http.Request) { r.Header.Del("X-Leftover-Token") }); w.Code != http.StatusForbidden {
		t.Errorf("why must need the token, got %d", w.Code)
	}
}

func TestExploreAndReveal(t *testing.T) {
	srv := testServer(t)
	home := srv.App.Scanner.Host.Home
	big := filepath.Join(home, "Videos", "trip.mov")
	os.MkdirAll(filepath.Dir(big), 0o755)
	if err := os.WriteFile(big, make([]byte, 3<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	// Before anything is explored, the window can't reveal arbitrary paths.
	reveal := `{"path":"` + filepath.ToSlash(filepath.Join(home, "Videos")) + `"}`
	if w := call(srv, "POST", "/api/reveal", reveal, nil); w.Code != http.StatusNotFound {
		t.Fatalf("reveal before explore: %d", w.Code)
	}
	w := call(srv, "POST", "/api/explore", `{}`, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"Videos"`) {
		t.Fatalf("explore home: %d %s", w.Code, w.Body.String())
	}
	if w := call(srv, "POST", "/api/explore", `{"path":"relative/dir"}`, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("relative path should be refused, got %d", w.Code)
	}
	if !srv.App.Explored(filepath.Join(home, "Videos")) {
		t.Fatal("explored folders should become revealable")
	}
}
