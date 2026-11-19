package rules

import "testing"

func TestHypothesisExamplesReviewOnly(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	r, ok := c.Get("hypothesis-examples")
	if !ok {
		t.Fatal("missing Hypothesis rule")
	}
	if r.Verdict != "review" || r.Verdict.Cleanable() {
		t.Fatal("saved failures must never be cleanable")
	}
	if r.Command != "" {
		t.Fatal("no automatic cleanup command")
	}
	if r.Project == nil {
		t.Fatal("expected project rule")
	}
	if len(r.Project.Dirs) != 1 || r.Project.Dirs[0] != ".hypothesis" || len(r.Project.Inside) != 1 || r.Project.Inside[0] != "examples" {
		t.Fatal("must require the examples database inside .hypothesis")
	}
}
