package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenWindow shows the app. It prefers a Chromium-based browser in app
// mode, which gives a plain window with no tabs or address bar, using a
// separate profile so it never touches your normal browsing. If none is
// installed it falls back to your default browser.
func OpenWindow(url, profile string) error {
	if bin := findChromium(); bin != "" {
		os.MkdirAll(profile, 0o700)
		args := []string{
			"--app=" + url,
			"--user-data-dir=" + profile,
			"--window-size=1200,800",
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-extensions",
			"--disable-sync",
			// Keep the window as quiet as the app: no update checks,
			// metrics, safe-browsing downloads or other background calls.
			"--disable-background-networking",
			"--disable-component-update",
			"--disable-domain-reliability",
			"--disable-client-side-phishing-detection",
			"--metrics-recording-only",
			"--no-pings",
		}
		cmd := exec.Command(bin, args...)
		if err := cmd.Start(); err == nil {
			go cmd.Wait()
			return nil
		}
	}
	return openDefault(url)
}

func findChromium() string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		for _, app := range []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium", "Arc"} {
			candidates = append(candidates,
				filepath.Join("/Applications", app+".app", "Contents", "MacOS", app),
				filepath.Join(os.Getenv("HOME"), "Applications", app+".app", "Contents", "MacOS", app))
		}
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"))
		}
	default:
		for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"} {
			if p, err := exec.LookPath(name); err == nil {
				return p
			}
		}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func openDefault(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
