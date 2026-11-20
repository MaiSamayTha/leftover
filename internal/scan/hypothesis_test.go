package scan

import (
	"path/filepath"
	"testing"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

func TestHypothesisMatcherRequiresDirectExamplesChild(t *testing.T) {
	rule, ok := rules.MustLoad().Get("hypothesis-examples")
	if !ok {
		t.Fatal("missing Hypothesis rule")
	}
	finder := projectFinder{rules: []*rules.Rule{rule}}
	finder.init()

	for _, tc := range []struct {
		name string
		file string
		want bool
	}{
		{"direct", filepath.Join("examples", "saved-example"), true},
		{"missing", "constants", false},
		{"nested", filepath.Join("other", "examples", "saved-example"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			dir := filepath.Join(project, ".hypothesis")
			write(t, filepath.Join(dir, tc.file), 1)

			match := finder.match(project, dir, ".hypothesis", nil)
			if got := match != nil && match.ID == rule.ID; got != tc.want {
				t.Errorf("match = %v, want %v", got, tc.want)
			}
		})
	}
}
