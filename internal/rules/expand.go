package rules

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Host describes the machine rules are expanded for. Tests build their own
// so they never look at the real home folder.
type Host struct {
	OS     string // runtime.GOOS value
	Home   string
	Getenv func(string) string
}

// CurrentHost describes the machine Leftover is running on.
func CurrentHost() Host {
	home, _ := os.UserHomeDir()
	return Host{OS: runtime.GOOS, Home: home, Getenv: os.Getenv}
}

func (h Host) env(k string) string {
	if h.Getenv == nil {
		return ""
	}
	return strings.TrimSpace(h.Getenv(k))
}

func (h Host) join(parts ...string) string {
	if h.OS == "windows" {
		return winJoin(parts...)
	}
	return filepath.ToSlash(filepath.Join(parts...))
}

// winJoin joins with backslashes regardless of the OS the code runs on, so
// Windows expansion can be tested on any machine.
func winJoin(parts ...string) string {
	var b []string
	for _, p := range parts {
		p = strings.ReplaceAll(p, "/", `\`)
		p = strings.Trim(p, `\`)
		if p != "" {
			b = append(b, p)
		}
	}
	out := strings.Join(b, `\`)
	if len(out) == 2 && out[1] == ':' {
		out += `\`
	}
	return out
}

// placeholder returns the folder a {name} placeholder stands for, or "" when
// it has no meaning on this OS.
func (h Host) placeholder(name string) string {
	xdg := func(v, fallback string) string {
		if p := h.env(v); p != "" {
			return p
		}
		return h.join(h.Home, fallback)
	}
	switch h.OS {
	case "windows":
		local := h.env("LOCALAPPDATA")
		if local == "" {
			local = winJoin(h.Home, "AppData", "Local")
		}
		roaming := h.env("APPDATA")
		if roaming == "" {
			roaming = winJoin(h.Home, "AppData", "Roaming")
		}
		temp := h.env("TEMP")
		if temp == "" {
			temp = winJoin(local, "Temp")
		}
		switch name {
		case "cache", "localappdata":
			return local
		case "data", "config", "appdata":
			return roaming
		case "temp":
			return temp
		}
	case "darwin":
		switch name {
		case "cache":
			return h.join(h.Home, "Library", "Caches")
		case "data", "config":
			return h.join(h.Home, "Library", "Application Support")
		case "xdgcache":
			return xdg("XDG_CACHE_HOME", ".cache")
		case "temp":
			if t := h.env("TMPDIR"); t != "" {
				return filepath.ToSlash(filepath.Clean(t))
			}
			return "/tmp"
		}
	default: // linux and other unixes
		switch name {
		case "cache", "xdgcache":
			return xdg("XDG_CACHE_HOME", ".cache")
		case "data":
			return xdg("XDG_DATA_HOME", ".local/share")
		case "config":
			return xdg("XDG_CONFIG_HOME", ".config")
		case "temp":
			if t := h.env("TMPDIR"); t != "" {
				return filepath.ToSlash(filepath.Clean(t))
			}
			return "/tmp"
		}
	}
	return ""
}

// Expand turns one catalogue path into a real path for this host. It
// understands a leading ~ and the placeholders {cache}, {xdgcache}, {data},
// {config}, {localappdata}, {appdata} and {temp}. It returns "" when the
// path can't be expanded here.
func (h Host) Expand(p string) string {
	if h.Home == "" {
		return ""
	}
	var head, rest string
	switch {
	case p == "~":
		return h.join(h.Home)
	case strings.HasPrefix(p, "~/"):
		head, rest = h.Home, p[2:]
	case strings.HasPrefix(p, "{"):
		end := strings.IndexByte(p, '}')
		if end < 0 {
			return ""
		}
		head = h.placeholder(p[1:end])
		if head == "" {
			return ""
		}
		rest = strings.TrimPrefix(p[end+1:], "/")
	case strings.HasPrefix(p, "/") && h.OS != "windows":
		return filepath.ToSlash(filepath.Clean(p))
	default:
		// Relative or unknown forms are never guessed at.
		return ""
	}
	if rest == "" {
		return h.join(head)
	}
	return h.join(head, rest)
}

// Locations returns every place a location rule points to on this host,
// honouring environment overrides.
func (r *Rule) Locations(h Host) []string {
	for _, e := range r.Env {
		v := h.env(e.Var)
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "~") {
			v = h.Expand(v)
		}
		if v == "" {
			continue
		}
		if e.Join != "" {
			v = h.join(v, e.Join)
		} else {
			v = h.join(v)
		}
		return []string{v}
	}
	keys := []string{"all", h.OS}
	if h.OS != "windows" {
		keys = append(keys, "unix")
	}
	seen := map[string]bool{}
	var out []string
	for _, k := range keys {
		for _, p := range r.Paths[k] {
			x := h.Expand(p)
			if x == "" || seen[x] {
				continue
			}
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
