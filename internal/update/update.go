// Package update checks GitHub for a newer Leftover and installs it.
//
// This is the only code in Leftover that uses the internet, and it only runs
// when someone asks: the Check for updates button or 'leftover update'. It
// asks GitHub's public releases API for the latest version and sends nothing
// but a "Leftover/<version>" user agent.
//
// Updates are only installed after two checks: checksums.txt must carry a
// valid Ed25519 signature from the release key built into this binary, and
// the download must match its SHA-256 in that file. A tampered or swapped
// download is refused.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases are published.
const Repo = "MaiSamayTha/leftover"

// apiBase and client are swapped out in tests.
var (
	apiBase = "https://api.github.com"
	client  = &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" && !allowHTTP {
				return errors.New("refusing a redirect away from https")
			}
			if len(via) > 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	allowHTTP = false // tests serve releases over plain http
)

// Method says how this copy of Leftover was installed, which decides how it
// can be updated.
type Method string

const (
	// App is Leftover.app from the dmg; the whole app is replaced.
	App Method = "app"
	// Binary is a single executable from an archive or install script.
	Binary Method = "binary"
	// Installer is the Windows installer; the new installer is run.
	Installer Method = "installer"
	// Brew, Scoop and Deb are updated by their package manager.
	Brew  Method = "brew"
	Scoop Method = "scoop"
	Deb   Method = "deb"
	// Dev is a build made from source, with no version to compare.
	Dev Method = "dev"
)

// Info is the answer to "is there a newer version, and how do I get it".
type Info struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Newer   bool   `json:"newer"`
	Method  Method `json:"method"`
	// CanApply means Update now can install it in place.
	CanApply bool `json:"can_apply"`
	// Command is what to run instead, for package-manager installs.
	Command string `json:"command,omitempty"`
	// Download is the file to get by hand when it can't be applied.
	Download string `json:"download,omitempty"`
	// Page is the release's page, with what's new.
	Page      string    `json:"page"`
	Published time.Time `json:"published"`

	release *release
	exe     string
}

type release struct {
	Tag       string    `json:"tag_name"`
	Page      string    `json:"html_url"`
	Published time.Time `json:"published_at"`
	Assets    []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func (r *release) asset(name string) (string, int64, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, a.Size, true
		}
	}
	return "", 0, false
}

// Check asks GitHub for the latest release and compares it with current.
func Check(ctx context.Context, current string) (*Info, error) {
	exe, _ := os.Executable()
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return check(ctx, current, exe)
}

func check(ctx context.Context, current, exe string) (*Info, error) {
	r, err := latest(ctx, current)
	if err != nil {
		return nil, err
	}
	info := &Info{
		Current: current, Latest: strings.TrimPrefix(r.Tag, "v"), Page: r.Page,
		Published: r.Published, release: r, exe: exe,
	}
	info.Method = detect(exe, current, runtime.GOOS)
	info.Newer = info.Method != Dev && newer(info.Latest, current)
	name := assetName(info.Method, runtime.GOOS, runtime.GOARCH)
	if u, _, ok := r.asset(name); ok {
		info.Download = u
	}
	switch info.Method {
	case Brew:
		info.Command = "brew upgrade leftover"
	case Scoop:
		info.Command = "scoop update leftover"
	case Deb:
		info.Command = "sudo apt install ./" + name
	}
	_, _, hasSig := r.asset("checksums.txt.sig")
	info.CanApply = info.Newer && info.Download != "" && hasSig && (info.Method == App || info.Method == Binary || info.Method == Installer)
	return info, nil
}

func latest(ctx context.Context, current string) (*release, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Leftover/"+current)
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("couldn't reach GitHub to check for updates; are you online?")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s while checking for updates; try again later", res.Status)
	}
	var r release
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 2<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("couldn't read GitHub's answer: %w", err)
	}
	if r.Tag == "" {
		return nil, errors.New("GitHub didn't name a latest release")
	}
	return &r, nil
}

// detect works out how this copy was installed from where it lives.
func detect(exe, version, goos string) Method {
	if version == "" || version == "dev" || !validVersion(version) {
		return Dev
	}
	p := strings.ReplaceAll(exe, `\`, "/")
	low := strings.ToLower(p)
	switch {
	case strings.Contains(p, "/Cellar/") || strings.Contains(p, "/homebrew/") || strings.Contains(p, "/linuxbrew/"):
		return Brew
	case strings.Contains(low, "/scoop/apps/"):
		return Scoop
	case goos == "darwin" && strings.HasSuffix(p, ".app/Contents/MacOS/Leftover"):
		return App
	case goos == "linux" && (p == "/usr/bin/leftover" || p == "/bin/leftover"):
		return Deb
	case goos == "windows":
		if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "unins000.exe")); err == nil {
			return Installer
		}
	}
	return Binary
}

// assetName is the release file each kind of install updates from.
func assetName(m Method, goos, goarch string) string {
	switch m {
	case App:
		return "Leftover-macos.zip"
	case Installer:
		return "Leftover-Setup.exe"
	case Deb:
		return "leftover-linux-" + goarch + ".deb"
	}
	name := goos
	if goos == "darwin" {
		name = "macos"
	}
	if goos == "windows" {
		return "leftover-windows-" + goarch + ".zip"
	}
	return "leftover-" + name + "-" + goarch + ".tar.gz"
}

func validVersion(v string) bool {
	_, ok := parse(v)
	return ok
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// newer reports whether version a is later than b.
func newer(a, b string) bool {
	x, ok1 := parse(a)
	y, ok2 := parse(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}
