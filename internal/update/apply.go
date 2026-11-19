package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Applied is an update that has been put in place, or for the Windows
// installer, downloaded, checked and ready to run.
type Applied struct {
	Version string `json:"version"`
	Method  Method `json:"method"`
	// Message says what happens next.
	Message string `json:"message"`
	restart func(open bool) error
}

// Restart opens the new version (for the Windows installer: installs it,
// then opens it). The caller should exit right after.
func (a *Applied) Restart() error { return a.run(true) }

// Finish completes the update without opening the app, for the command
// line. Only the Windows installer has anything left to do.
func (a *Applied) Finish() error {
	if a.Method != Installer {
		return nil
	}
	return a.run(false)
}

func (a *Applied) run(open bool) error {
	if a.restart == nil {
		return nil
	}
	return a.restart(open)
}

// Apply downloads the update Check found, verifies it and installs it.
// Nothing is changed unless both the signature and the checksum match.
func Apply(ctx context.Context, info *Info) (*Applied, error) {
	if info == nil || !info.CanApply || info.release == nil {
		return nil, errors.New("there's no update that can be installed from here")
	}
	name := assetName(info.Method, runtime.GOOS, runtime.GOARCH)
	sums, err := verifiedSums(ctx, info.release, info.Current)
	if err != nil {
		return nil, err
	}
	want, ok := sums[name]
	if !ok {
		return nil, fmt.Errorf("%s isn't listed in the release's checksums, so it can't be verified", name)
	}
	url, size, _ := info.release.asset(name)
	if size > 512<<20 {
		return nil, errors.New("the update is unexpectedly large; download it from the release page instead")
	}

	stage, err := os.MkdirTemp("", "leftover-update-")
	if err != nil {
		return nil, err
	}
	file := filepath.Join(stage, name)
	got, err := download(ctx, url, file, info.Current)
	if err != nil {
		os.RemoveAll(stage)
		return nil, err
	}
	if got != want {
		os.RemoveAll(stage)
		return nil, errors.New("the download doesn't match the release's checksum, so it wasn't installed")
	}

	out := &Applied{Version: info.Latest, Method: info.Method}
	switch info.Method {
	case Binary:
		err = replaceBinary(file, info.exe)
		os.RemoveAll(stage)
		out.Message = "Updated to " + info.Latest + "."
		exe := info.exe
		out.restart = func(bool) error { return exec.Command(exe, "app").Start() }
	case App:
		var appDir string
		appDir, err = replaceApp(file, info.exe)
		os.RemoveAll(stage)
		out.Message = "Updated to " + info.Latest + "."
		out.restart = func(bool) error { return exec.Command("open", "-n", appDir).Start() }
	case Installer:
		// The installer replaces the running files, so it runs as the
		// restart step: it closes Leftover, installs and opens it again.
		out.Message = "Leftover will close, install " + info.Latest + " and open again."
		out.restart = func(open bool) error {
			args := []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/CLOSEAPPLICATIONS"}
			if open {
				args = append(args, "/LAUNCH=1")
			}
			return exec.Command(file, args...).Start()
		}
	default:
		err = errors.New("this copy of Leftover is updated by its package manager")
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// verifiedSums fetches checksums.txt and its signature, checks the
// signature against the release key and returns name -> sha256.
func verifiedSums(ctx context.Context, r *release, current string) (map[string]string, error) {
	sumURL, _, ok1 := r.asset("checksums.txt")
	sigURL, _, ok2 := r.asset("checksums.txt.sig")
	if !ok1 || !ok2 {
		return nil, errors.New("this release isn't signed, so Leftover won't install it; download it from the release page instead")
	}
	sums, err := fetch(ctx, sumURL, current, 1<<20)
	if err != nil {
		return nil, err
	}
	sigText, err := fetch(ctx, sigURL, current, 4<<10)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigText)))
	if err != nil || !verify(sums, sig) {
		return nil, errors.New("the release's signature doesn't check out, so nothing was installed")
	}
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 {
			out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	return out, nil
}

// verify checks sig against every trusted release key.
func verify(msg, sig []byte) bool {
	for _, k := range publicKeys {
		raw, err := base64.StdEncoding.DecodeString(k)
		if err == nil && len(raw) == ed25519.PublicKeySize && ed25519.Verify(raw, msg, sig) {
			return true
		}
	}
	return false
}

func get(ctx context.Context, url, current string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Leftover/"+current)
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("the download failed; check your connection and try again")
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("the download failed: %s", res.Status)
	}
	return res, nil
}

func fetch(ctx context.Context, url, current string, limit int64) ([]byte, error) {
	res, err := get(ctx, url, current)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("a release file was larger than expected")
	}
	return b, nil
}

// download saves url to path and returns its SHA-256.
func download(ctx context.Context, url, path, current string) (string, error) {
	res, err := get(ctx, url, current)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, 512<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("the download failed: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// replaceBinary takes the executable out of a release archive and puts it
// where the running one is. On Windows a running program can't be
// overwritten but can be renamed, so the old one is moved aside first.
func replaceBinary(archive, exe string) error {
	want := "leftover"
	if strings.HasSuffix(strings.ToLower(exe), ".exe") {
		want = "leftover.exe"
	}
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".leftover-update-")
	if err != nil {
		return fmt.Errorf("Leftover can't write to %s, so it can't update itself there; download the new version instead", filepath.Dir(exe))
	}
	defer os.Remove(tmp.Name())
	if err := extract(archive, want, tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Rename(tmp.Name(), exe)
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	return nil
}

// extract copies the file called name out of a .tar.gz or .zip.
func extract(archive, name string, w io.Writer) error {
	if strings.HasSuffix(archive, ".zip") {
		z, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			if filepath.Base(f.Name) == name && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				defer rc.Close()
				_, err = io.Copy(w, io.LimitReader(rc, 256<<20))
				return err
			}
		}
		return fmt.Errorf("%s wasn't in the download", name)
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s wasn't in the download", name)
		}
		if err != nil {
			return err
		}
		if filepath.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
			_, err = io.Copy(w, io.LimitReader(tr, 256<<20))
			return err
		}
	}
}

// replaceApp swaps Leftover.app for the one in the downloaded zip. The new
// app is unpacked next to the old one first and checked with codesign, so
// the swap itself is two renames on the same disk.
func replaceApp(zipPath, exe string) (string, error) {
	appDir := strings.TrimSuffix(exe, "/Contents/MacOS/Leftover")
	parent := filepath.Dir(appDir)
	stage, err := os.MkdirTemp(parent, ".leftover-update-")
	if err != nil {
		return "", fmt.Errorf("Leftover can't write to %s, so it can't update itself there; download the new version instead", parent)
	}
	defer os.RemoveAll(stage)
	if out, err := exec.Command("ditto", "-x", "-k", zipPath, stage).CombinedOutput(); err != nil {
		return "", fmt.Errorf("couldn't unpack the update: %s", strings.TrimSpace(string(out)))
	}
	fresh := filepath.Join(stage, "Leftover.app")
	if _, err := os.Stat(filepath.Join(fresh, "Contents", "MacOS", "Leftover")); err != nil {
		return "", errors.New("the download didn't contain Leftover.app")
	}
	if out, err := exec.Command("codesign", "--verify", "--strict", fresh).CombinedOutput(); err != nil {
		return "", fmt.Errorf("the new app's code signature doesn't check out: %s", strings.TrimSpace(string(out)))
	}
	old := filepath.Join(stage, "old.app")
	if err := os.Rename(appDir, old); err != nil {
		return "", fmt.Errorf("couldn't replace %s: %w", appDir, err)
	}
	if err := os.Rename(fresh, appDir); err != nil {
		os.Rename(old, appDir)
		return "", fmt.Errorf("couldn't replace %s: %w", appDir, err)
	}
	return appDir, nil
}
