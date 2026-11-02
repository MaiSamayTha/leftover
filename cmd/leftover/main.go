// Leftover finds the disk space your developer tools leave behind, explains
// what each piece is, and moves what's safe into a vault you can restore from.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

type command struct {
	name  string
	args  string
	short string
	run   func(args []string) error
}

var commands []command

func register(c command) { commands = append(commands, c) }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "leftover:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return openApp(nil)
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return nil
	case "-v", "--version":
		fmt.Println("leftover", version)
		return nil
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:])
		}
	}
	usage(os.Stderr)
	return fmt.Errorf("unknown command %q", args[0])
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Leftover finds the space your dev tools leave behind, explains it, and clears it safely.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  leftover                         open the app")
	sorted := append([]command(nil), commands...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].name < sorted[j].name })
	for _, c := range sorted {
		fmt.Fprintf(w, "  leftover %s\n", pad(strings.TrimSpace(c.name+" "+c.args), 24)+c.short)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'leftover <command> -h' for a command's options.")
}

// openApp starts the desktop window. Until the window lands this prints
// a hint so a double-click doesn't silently do nothing.
var openApp = func(args []string) error {
	fmt.Println("Run 'leftover scan' to see what's taking space.")
	return nil
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("leftover "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// interruptible returns a context cancelled by Ctrl-C.
func interruptible() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	register(command{name: "scan", short: "find and explain what's using space", run: cmdScan})
}

// filter narrows a scan's items for display or cleaning.
type filter struct {
	all     bool
	minSize int64
	idle    int
	tool    string
}

func (f filter) keep(it scan.Item) bool {
	if !f.all && !it.Cleanable() && it.Bytes < 100e6 {
		return false // small review/keep items are noise unless asked for
	}
	if it.Bytes < f.minSize {
		return false
	}
	if f.idle > 0 && it.IdleDays < f.idle {
		return false
	}
	if f.tool != "" && !strings.Contains(strings.ToLower(it.Tool+" "+it.Rule), strings.ToLower(f.tool)) {
		return false
	}
	return true
}

func addFilterFlags(fs *flag.FlagSet, f *filter, minSize *string) {
	fs.BoolVar(&f.all, "all", false, "show small items Leftover won't clean, too")
	fs.StringVar(minSize, "min", "1MB", "hide items smaller than this (e.g. 500MB)")
	fs.IntVar(&f.idle, "idle", 0, "only items whose project hasn't changed in this many days")
	fs.StringVar(&f.tool, "tool", "", "only items from a tool (npm, python, xcode, ...)")
}

func cmdScan(args []string) error {
	fs := newFlags("scan")
	var f filter
	var minSize string
	asJSON := fs.Bool("json", false, "print the full result as JSON")
	quiet := fs.Bool("q", false, "don't show progress")
	addFilterFlags(fs, &f, &minSize)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var err error
	if f.minSize, err = parseSize(minSize); err != nil {
		return err
	}

	a, err := app.Open()
	if err != nil {
		return err
	}
	ctx, stop := interruptible()
	defer stop()

	showProgress := !*asJSON && !*quiet && useColor
	res, emptied, err := a.Scan(ctx, func(p scan.Progress) {
		if showProgress {
			fmt.Fprintf(os.Stderr, "\r\x1b[2K%s %s folders, %d found", dim("Scanning"), fmt.Sprint(p.Folders), p.Found)
		}
	})
	if showProgress {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("scan cancelled")
		}
		return err
	}
	if *asJSON {
		return printJSON(res)
	}
	printScan(res, f)
	if len(emptied) > 0 {
		var n int64
		for _, e := range emptied {
			n += e.Bytes
		}
		fmt.Printf("\n%s\n", dim(fmt.Sprintf("Emptied %d expired vault item(s), %s freed.", len(emptied), human(n))))
	}
	return nil
}

func printScan(res *scan.Result, f filter) {
	groups := []struct {
		title    string
		verdicts []rules.Verdict
	}{
		{"Safe to clear", []rules.Verdict{rules.Rebuild, rules.Redownload}},
		{"Worth a look", []rules.Verdict{rules.Review}},
		{"Your data (never cleaned)", []rules.Verdict{rules.Keep}},
	}
	var safe int64
	for _, g := range groups {
		var items []scan.Item
		var total int64
		for _, it := range res.Items {
			if !f.keep(it) {
				continue
			}
			for _, v := range g.verdicts {
				if it.Verdict == v {
					items = append(items, it)
					total += it.Bytes
				}
			}
		}
		if len(items) == 0 {
			continue
		}
		if g.verdicts[0] == rules.Rebuild {
			safe = total
		}
		fmt.Printf("\n%s  %s\n", bold(g.title), dim(human(total)))
		for _, it := range items {
			where := it.Paths[0]
			if it.Project != "" {
				where = it.Project
			}
			extra := ""
			if len(it.Paths) > 1 {
				extra = fmt.Sprintf(" (%d folders)", len(it.Paths))
			}
			idle := ""
			if it.Project != "" && it.IdleDays >= 30 {
				idle = dim("  untouched " + ago(it.IdleDays))
			}
			fmt.Printf("  %s %s  %s  %s  %s%s%s\n", verdictMark(it.Verdict), lpad(human(it.Bytes), 8),
				dim(it.ID), pad(clip(it.Name+extra, 28), 28), clip(tilde(where), 48), idle, noteMark(it))
		}
	}
	fmt.Printf("\n%s in %d folders, %.1fs.", bold(human(safe)+" can be cleared safely"), res.Folders, res.Took.Seconds())
	if safe > 0 {
		fmt.Printf(" Run %s to review and clear it.\n", bold("leftover clean"))
	} else {
		fmt.Println()
	}
}

func noteMark(it scan.Item) string {
	if len(it.Notes) == 0 {
		return ""
	}
	return paint("33", "  !")
}
