package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

func TestFilterAllIncludesItemsBelowMin(t *testing.T) {
	item := scan.Item{
		Verdict: rules.Rebuild,
		Paths:   []string{"a"},
		Size:    scan.Size{Bytes: 500},
	}

	if (filter{minSize: 1_000_000}).keep(item) {
		t.Fatal("item below --min should be hidden without --all")
	}
	if !(filter{all: true, minSize: 1_000_000}).keep(item) {
		t.Fatal("--all should include items below --min")
	}
}

func TestHiddenByAllCountsFoldersAndBytes(t *testing.T) {
	res := &scan.Result{Items: []scan.Item{
		{Verdict: rules.Rebuild, Paths: []string{"a", "b"}, Size: scan.Size{Bytes: 500}},
		{Verdict: rules.Rebuild, Paths: []string{"c"}, Size: scan.Size{Bytes: 1_500_000}},
	}}

	res.Items = append(res.Items, scan.Item{
		Verdict: rules.Review,
		Paths:   []string{"d"},
		Size:    scan.Size{Bytes: 5_000_000},
	})

	folders, bytes := hiddenByAll(res, filter{minSize: 10_000_000})
	if folders != 4 || bytes != 6_500_500 {
		t.Fatalf("got %d folders and %d bytes, want 4 and 6,500,500", folders, bytes)
	}

	folders, bytes = hiddenByAll(res, filter{all: true, minSize: 10_000_000})
	if folders != 0 || bytes != 0 {
		t.Fatalf("--all should not hide anything, got %d folders and %d bytes", folders, bytes)
	}
}

func TestPrintScanReportsHiddenItems(t *testing.T) {
	res := &scan.Result{
		Folders: 5,
		Items: []scan.Item{
			{Verdict: rules.Rebuild, Paths: []string{"a", "b", "c", "d", "e"}, Size: scan.Size{Bytes: 500}},
		},
	}

	oldColor := useColor
	useColor = false
	t.Cleanup(func() { useColor = oldColor })

	out := captureStdout(t, func() { printScan(res, filter{minSize: 1_000_000}) })

	for _, want := range []string{"No folders shown", "5 smaller folders", "500 B", "--all"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "can be cleared safely") {
		t.Errorf("output should not claim there is anything to clear:\n%s", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
