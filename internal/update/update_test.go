package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.1.2", "0.1.1", true}, {"0.1.10", "0.1.9", true}, {"1.0.0", "0.9.9", true},
		{"0.1.1", "0.1.1", false}, {"0.1.0", "0.1.1", false}, {"v0.2.0", "0.1.9", true},
		{"0.2.0", "dev", false}, {"junk", "0.1.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestDetect(t *testing.T) {
	for _, c := range []struct {
		exe, version, goos string
		want               Method
	}{
		{"/Applications/Leftover.app/Contents/MacOS/Leftover", "0.1.1", "darwin", App},
		{"/opt/homebrew/Cellar/leftover/0.1.1/bin/leftover", "0.1.1", "darwin", Brew},
		{"/home/linuxbrew/.linuxbrew/Cellar/leftover/0.1.1/bin/leftover", "0.1.1", "linux", Brew},
		{"/usr/bin/leftover", "0.1.1", "linux", Deb},
		{"/home/sam/.local/bin/leftover", "0.1.1", "linux", Binary},
		{`C:\Users\sam\scoop\apps\leftover\current\leftover.exe`, "0.1.1", "windows", Scoop},
		{"/Applications/Leftover.app/Contents/MacOS/Leftover", "dev", "darwin", Dev},
	} {
		if got := detect(c.exe, c.version, c.goos); got != c.want {
			t.Errorf("detect(%q) = %s, want %s", c.exe, got, c.want)
		}
	}
}

// fakeRelease serves a release whose archive holds a new "leftover", with
// checksums signed by key. tamper changes the archive after signing.
func fakeRelease(t *testing.T, key ed25519.PrivateKey, tamper bool) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho new\n")
	tw.WriteHeader(&tar.Header{Name: "./leftover", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	gz.Close()
	archive := buf.Bytes()
	name := assetName(Binary, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, sums))
	if tamper {
		archive = append([]byte(nil), archive...)
		archive[len(archive)-1] ^= 0xff
	}
	files := map[string][]byte{name: archive, "checksums.txt": sums, "checksums.txt.sig": []byte(sig)}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/"+Repo+"/releases/latest" {
			var assets []map[string]any
			for n, b := range files {
				assets = append(assets, map[string]any{"name": n, "browser_download_url": srv.URL + "/dl/" + n, "size": len(b)})
			}
			json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.9.0", "html_url": "https://example.com/r", "assets": assets})
			return
		}
		if b, ok := files[strings.TrimPrefix(r.URL.Path, "/dl/")]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	oldBase, oldAllow := apiBase, allowHTTP
	apiBase, allowHTTP = srv.URL, true
	t.Cleanup(func() { apiBase, allowHTTP = oldBase, oldAllow })
}

func trustKey(t *testing.T) ed25519.PrivateKey {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := publicKeys
	publicKeys = []string{base64.StdEncoding.EncodeToString(pub)}
	t.Cleanup(func() { publicKeys = old })
	return priv
}

func fakeExe(t *testing.T) string {
	exe := filepath.Join(t.TempDir(), "leftover")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestUpdateBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("archive layout is the Unix one")
	}
	fakeRelease(t, trustKey(t), false)
	exe := fakeExe(t)
	info, err := check(context.Background(), "0.1.1", exe)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Newer || !info.CanApply || info.Method != Binary || info.Latest != "0.9.0" {
		t.Fatalf("info = %+v", info)
	}
	if _, err := Apply(context.Background(), info); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); !strings.Contains(string(b), "echo new") {
		t.Fatalf("executable wasn't replaced: %q", b)
	}
}

func TestRefusesTamperedDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("archive layout is the Unix one")
	}
	fakeRelease(t, trustKey(t), true)
	exe := fakeExe(t)
	info, err := check(context.Background(), "0.1.1", exe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), info); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want a checksum error, got %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("a tampered download must not replace anything")
	}
}

func TestRefusesWrongSigningKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("archive layout is the Unix one")
	}
	trustKey(t)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	fakeRelease(t, other, false)
	exe := fakeExe(t)
	info, err := check(context.Background(), "0.1.1", exe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), info); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("want a signature error, got %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("an update signed by an unknown key must not replace anything")
	}
}

func TestUpToDate(t *testing.T) {
	fakeRelease(t, trustKey(t), false)
	info, err := check(context.Background(), "0.9.0", fakeExe(t))
	if err != nil {
		t.Fatal(err)
	}
	if info.Newer || info.CanApply {
		t.Fatalf("same version shouldn't offer an update: %+v", info)
	}
	if _, err := Apply(context.Background(), info); err == nil {
		t.Fatal("Apply should refuse when there's nothing newer")
	}
}
