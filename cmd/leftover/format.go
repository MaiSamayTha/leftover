package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

// human prints a byte count the way macOS and most people count: 1 GB is
// a thousand million bytes.
func human(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	suffix := []string{"KB", "MB", "GB", "TB", "PB"}[exp]
	if v >= 100 {
		return fmt.Sprintf("%.0f %s", v, suffix)
	}
	return fmt.Sprintf("%.1f %s", v, suffix)
}

// parseSize understands "500MB", "2GB", "1.5 gb" and plain bytes.
func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		m      int64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3}, {"T", 1e12}, {"G", 1e9}, {"M", 1e6}, {"K", 1e3}, {"B", 1}} {
		if strings.HasSuffix(s, u.suffix) {
			mult = u.m
			s = strings.TrimSuffix(s, u.suffix)
			break
		}
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil || f < 0 {
		return 0, fmt.Errorf("can't read size %q (try 500MB or 2GB)", s)
	}
	return int64(f * float64(mult)), nil
}

func ago(days int) string {
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 60:
		return fmt.Sprintf("%d days ago", days)
	case days < 730:
		return fmt.Sprintf("%d months ago", days/30)
	}
	return fmt.Sprintf("%d years ago", days/365)
}

func until(t, now time.Time) string {
	d := t.Sub(now)
	if d <= 0 {
		return "expired"
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh left", int(d.Hours())+1)
	}
	days := int((d + 24*time.Hour - 1) / (24 * time.Hour))
	return fmt.Sprintf("%d days left", days)
}

// tilde shortens a path under the home folder for display.
func tilde(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(p, home) {
		rest := p[len(home):]
		if rest == "" || rest[0] == '/' || rest[0] == '\\' {
			return "~" + rest
		}
	}
	return p
}

var useColor = func() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}()

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string  { return paint("1", s) }
func dim(s string) string   { return paint("2", s) }
func green(s string) string { return paint("32", s) }

func verdictMark(v rules.Verdict) string {
	switch v {
	case rules.Rebuild, rules.Redownload:
		return paint("32", "●")
	case rules.Review:
		return paint("33", "●")
	}
	return paint("34", "●")
}

func pad(s string, n int) string {
	w := len([]rune(s))
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func lpad(s string, n int) string {
	w := len([]rune(s))
	if w >= n {
		return s
	}
	return strings.Repeat(" ", n-w) + s
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 2 {
		return string(r[:n])
	}
	// Keep the end of paths, which is the interesting part.
	return "…" + string(r[len(r)-n+1:])
}
