package rules

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestCatalogLoads(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Rules) < 30 {
		t.Fatalf("expected a full catalogue, got %d rules", len(c.Rules))
	}
	for _, r := range c.Rules {
		if r.Verdict.Cleanable() && !strings.HasPrefix(r.Source, "https://") {
			t.Errorf("%s is cleanable without a source", r.ID)
		}
		if got, ok := c.Get(r.ID); !ok || got.ID != r.ID {
			t.Errorf("index broken for %s", r.ID)
		}
	}
}

func TestRejectsUnsourcedCleanableRule(t *testing.T) {
	fsys := fstest.MapFS{"x.json": {Data: []byte(`{"rules":[{
		"id":"x","tool":"t","kind":"k","name":"n","what":"w",
		"verdict":"rebuild","comeback":"c","paths":{"all":["~/x"]}}]}`)}}
	if _, err := LoadFS(fsys); err == nil {
		t.Fatal("a rebuild rule without a source must be rejected")
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	fsys := fstest.MapFS{"x.json": {Data: []byte(`{"rules":[{
		"id":"x","tool":"t","kind":"k","name":"n","what":"w","verdict":"keep",
		"paths":{"all":["~/x"]},"typo":true}]}`)}}
	if _, err := LoadFS(fsys); err == nil {
		t.Fatal("unknown fields must be rejected so typos don't silently drop settings")
	}
}

func TestRejectsDuplicateIDs(t *testing.T) {
	rule := `{"id":"x","tool":"t","kind":"k","name":"n","what":"w","verdict":"keep","paths":{"all":["~/x"]}}`
	fsys := fstest.MapFS{"x.json": {Data: []byte(`{"rules":[` + rule + `,` + rule + `]}`)}}
	if _, err := LoadFS(fsys); err == nil {
		t.Fatal("duplicate ids must be rejected")
	}
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestExpand(t *testing.T) {
	mac := Host{OS: "darwin", Home: "/Users/sam", Getenv: env(nil)}
	lin := Host{OS: "linux", Home: "/home/sam", Getenv: env(map[string]string{"XDG_CACHE_HOME": "/fast/cache"})}
	win := Host{OS: "windows", Home: `C:\Users\sam`, Getenv: env(map[string]string{"LOCALAPPDATA": `C:\Users\sam\AppData\Local`})}

	cases := []struct {
		h    Host
		in   string
		want string
	}{
		{mac, "{cache}/pip", "/Users/sam/Library/Caches/pip"},
		{mac, "{xdgcache}/uv", "/Users/sam/.cache/uv"},
		{mac, "~/.npm/_cacache", "/Users/sam/.npm/_cacache"},
		{lin, "{cache}/pip", "/fast/cache/pip"},
		{lin, "{data}/pnpm/store", "/home/sam/.local/share/pnpm/store"},
		{lin, "/usr/share/ollama", "/usr/share/ollama"},
		{win, "{localappdata}/pip/Cache", `C:\Users\sam\AppData\Local\pip\Cache`},
		{win, "~/.cache/huggingface/hub", `C:\Users\sam\.cache\huggingface\hub`},
		{win, "{appdata}/Code/Cache", `C:\Users\sam\AppData\Roaming\Code\Cache`},
		{win, "/etc/x", ""},
		{mac, "relative/path", ""},
		{mac, "{nope}/x", ""},
	}
	for _, c := range cases {
		if got := c.h.Expand(c.in); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.h.OS, c.in, got, c.want)
		}
	}
}

func TestLocationsPreferEnv(t *testing.T) {
	c := MustLoad()
	hf, _ := c.Get("hf-hub")
	h := Host{OS: "linux", Home: "/home/sam", Getenv: env(map[string]string{"HF_HOME": "/data/hf"})}
	got := hf.Locations(h)
	if len(got) != 1 || got[0] != "/data/hf/hub" {
		t.Fatalf("got %v", got)
	}
	h.Getenv = env(nil)
	if got := hf.Locations(h); len(got) != 1 || got[0] != "/home/sam/.cache/huggingface/hub" {
		t.Fatalf("default location: got %v", got)
	}
}

func TestLocationsAreOSSpecific(t *testing.T) {
	c := MustLoad()
	dd, _ := c.Get("xcode-derived-data")
	if got := dd.Locations(Host{OS: "linux", Home: "/home/sam"}); len(got) != 0 {
		t.Fatalf("Xcode paths should not exist on Linux, got %v", got)
	}
	if got := dd.Locations(Host{OS: "darwin", Home: "/Users/sam"}); len(got) != 1 {
		t.Fatalf("Xcode paths should exist on macOS, got %v", got)
	}
}
