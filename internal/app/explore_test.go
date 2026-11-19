package app

import (
	"context"
	"path/filepath"
	"testing"
)

func TestExplore(t *testing.T) {
	a, home := testApp(t)
	put(t, filepath.Join(home, "code/web/node_modules/big.js"), 3<<20)
	put(t, filepath.Join(home, "Videos/trip.mov"), 5<<20)
	if _, _, err := a.Scan(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	v, err := a.Explore(context.Background(), home, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Cached || len(v.Entries) < 2 || v.Entries[0].Name != "Videos" {
		t.Fatalf("home view = %+v", v)
	}
	var code ExploreEntry
	for _, e := range v.Entries {
		if e.Name == "code" {
			code = e
		}
	}
	if code.Clearable < 3<<20 {
		t.Fatalf("code should show its node_modules as clearable, got %+v", code)
	}
	// Drilling down reuses the measured tree and names what it finds.
	w, err := a.Explore(context.Background(), filepath.Join(home, "code/web"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Cached || len(w.Entries) == 0 || w.Entries[0].Rule == nil || w.Entries[0].Rule.ID != "node-modules" {
		t.Fatalf("code/web view = %+v", w)
	}
	if !a.Explored(filepath.Join(home, "code/web/node_modules")) {
		t.Fatal("explored folders should be known to the window")
	}
}
