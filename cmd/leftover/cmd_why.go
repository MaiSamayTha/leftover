package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/rules"
)

func init() {
	register(command{name: "why", args: "<path>", short: "explain what a folder is and whether it's safe", run: cmdWhy})
}

func cmdWhy(args []string) error {
	fs := newFlags("why")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: leftover why <path>")
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	r, matched := a.Explain(fs.Arg(0))
	if *asJSON {
		if r == nil {
			return printJSON(map[string]any{"path": fs.Arg(0), "known": false})
		}
		return printJSON(map[string]any{"path": fs.Arg(0), "known": true, "matched": matched, "rule": r})
	}
	if r == nil {
		fmt.Printf("Leftover doesn't know what %s is, so it treats it as yours.\n", tilde(fs.Arg(0)))
		fmt.Println(dim("If it belongs to a dev tool, a rule for it would be a welcome contribution."))
		return nil
	}
	printRule(r, matched)
	return nil
}

func printRule(r *rules.Rule, matched string) {
	fmt.Printf("%s %s  %s\n", verdictMark(r.Verdict), bold(r.Name), dim(r.Tool+" · "+r.Kind))
	if matched != "" {
		fmt.Println(dim(tilde(matched)))
	}
	fmt.Println()
	fmt.Println(wrap(r.What, 72))
	fmt.Println()
	fmt.Printf("%s %s\n", bold("Verdict:"), r.Verdict.Label())
	if r.Comeback != "" {
		fmt.Println(wrap(r.Comeback, 72))
	}
	if !r.Verdict.Cleanable() {
		fmt.Println(dim("Leftover shows this but never cleans it."))
	}
	if r.Command != "" {
		fmt.Printf("\n%s %s\n", bold("Tool's own command:"), r.Command)
	}
	if r.Source != "" {
		fmt.Printf("%s %s\n", bold("Source:"), r.Source)
	}
}

// wrap breaks text into lines no wider than width.
func wrap(s string, width int) string {
	var b strings.Builder
	line := 0
	for i, w := range strings.Fields(s) {
		if i > 0 {
			if line+1+len(w) > width {
				b.WriteByte('\n')
				line = 0
			} else {
				b.WriteByte(' ')
				line++
			}
		}
		b.WriteString(w)
		line += len(w)
	}
	return b.String()
}
