package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MaiSamayTha/leftover/internal/app"
)

func init() {
	register(command{name: "explore", args: "[path]", short: "see what any folder holds, biggest first", run: cmdExplore})
}

func cmdExplore(args []string) error {
	fs := newFlags("explore")
	limit := fs.Int("n", 20, "how many entries to show")
	asJSON := fs.Bool("json", false, "print as JSON")
	// Flags may come after the path too: leftover explore ~/code --json.
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(pos) > 1 {
		return fmt.Errorf("explore takes one folder, got %d", len(pos))
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	path := a.Scanner.Host.Home
	if len(pos) == 1 {
		path = pos[0]
	}
	ctx, stop := interruptible()
	defer stop()
	var progress func(int64, string)
	if !*asJSON && isTerminal(os.Stderr) {
		progress = func(n int64, cur string) {
			fmt.Fprintf(os.Stderr, "\r\033[K%s", dim(fmt.Sprintf("Measuring… %d folders  %s", n, clip(tilde(cur), 60))))
		}
	}
	v, err := a.Explore(ctx, path, true, progress)
	if progress != nil {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(v)
	}

	fmt.Printf("%s  %s\n", bold(tilde(v.Path)), dim(fmt.Sprintf("%s in %s files, measured %s", human(v.Bytes), commas(v.Files), took(v.Took))))
	if v.Rule != nil {
		fmt.Println(dim(fmt.Sprintf("Leftover knows this as %s (%s): %s.", v.Rule.Name, v.Rule.Tool, v.Rule.Verdict.Label())))
	}
	if v.Clearable > 0 {
		fmt.Println(green(fmt.Sprintf("%s in here is safe to clear, from your last scan.", human(v.Clearable))))
	}
	fmt.Println()
	var biggest int64 = 1
	if len(v.Entries) > 0 && v.Entries[0].Bytes > 0 {
		biggest = v.Entries[0].Bytes
	}
	shown := v.Entries
	if *limit > 0 && len(shown) > *limit {
		shown = shown[:*limit]
	}
	var hidden int64 = v.Rest
	for _, e := range v.Entries[len(shown):] {
		hidden += e.Bytes
	}
	for _, e := range shown {
		name := e.Name
		if e.Dir {
			name += string(os.PathSeparator)
		}
		fmt.Printf("  %s %s  %s  %s\n", bar(e.Bytes, biggest, 12), lpad(human(e.Bytes), 8), pad(clip(name, 32), 32), note(e))
	}
	if hidden > 0 {
		fmt.Printf("  %s %s  %s\n", strings.Repeat(" ", 12), lpad(human(hidden), 8), dim("in smaller items"))
	}
	if v.Unread > 0 {
		fmt.Println()
		fmt.Println(dim(fmt.Sprintf("%d folder(s) couldn't be read, so the real total may be higher.", v.Unread)))
	}
	return nil
}

// note says what Leftover knows about an entry.
func note(e app.ExploreEntry) string {
	switch {
	case e.Rule != nil && e.Rule.At == e.Path:
		return verdictMark(e.Rule.Verdict) + " " + dim(e.Rule.Name+" · "+e.Rule.Verdict.Label())
	case e.Rule != nil:
		return dim("inside " + e.Rule.Name)
	case e.Clearable > 0:
		return green(human(e.Clearable) + " safe to clear inside")
	}
	return ""
}

func bar(n, max int64, width int) string {
	full := int(float64(n) / float64(max) * float64(width))
	if full < 1 && n > 0 {
		full = 1
	}
	return paint("36", strings.Repeat("█", full)) + strings.Repeat(" ", width-full)
}

func took(d time.Duration) string {
	if d < time.Second {
		return "in under a second"
	}
	return "in " + d.Round(100*time.Millisecond).String()
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func commas(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
