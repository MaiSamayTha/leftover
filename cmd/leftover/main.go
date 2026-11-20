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
	err := run(os.Args[1:])
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "leftover:", err)
	}
	os.Exit(exitCode(err))
}

func run(args []string) error {
	// Older macOS versions pass -psn_<id> to apps opened from Finder.
	if len(args) > 0 && strings.HasPrefix(args[0], "-psn_") {
		args = args[1:]
	}
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
	case "__complete": // called by the completion scripts; not listed
		return cmdComplete(args[1:])
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:])
		}
	}
	usage(os.Stderr)
	return usageError("unknown command %q", args[0])
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Leftover finds the space your dev tools leave behind, explains it, and clears it safely.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	sorted := append([]command(nil), commands...)
	width := 0
	for _, c := range sorted {
		if n := len(strings.TrimSpace(c.name + " " + c.args)); n > width {
			width = n
		}
	}
	width += 3
	fmt.Fprintf(w, "  leftover %s%s\n", pad("", width), "open the app")
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].name < sorted[j].name })
	for _, c := range sorted {
		fmt.Fprintf(w, "  leftover %s\n", pad(strings.TrimSpace(c.name+" "+c.args), width)+c.short)
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

func newFlags(name string) flagSet {
	fs := flag.NewFlagSet("leftover "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return flagSet{fs}
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

func addFilterFlags(fs flagSet, f *filter, minSize *string) {
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
		return usageError("--min: %v", err)
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
			return &exitErr{exitInterrupted, errors.New("scan cancelled")}
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
	var shown, hidden int
	var hiddenBytes int64
	for _, it := range res.Items {
		if f.keep(it) {
			shown++
		} else if it.Cleanable() {
			hidden++
			hiddenBytes += it.Bytes
		}
	}
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
			for i, d := range it.Details {
				if i == 5 {
					fmt.Println(dim(fmt.Sprintf("%15s and %d more", "", len(it.Details)-5)))
					break
				}
				shared := ""
				if d.Shared {
					shared = " (shares layers)"
				}
				fmt.Printf("%s %s  %s%s\n", dim(fmt.Sprintf("%13s└", "")), lpad(human(d.Bytes), 8), d.Name, dim(shared))
			}
		}
	}
	took := fmt.Sprintf("Looked through %d folders in %.1fs.", res.Folders, res.Took.Seconds())
	switch {
	case shown == 0 && hidden > 0:
		what := fmt.Sprintf("%d smaller items (%s in all) are hidden; show them", hidden, human(hiddenBytes))
		if hidden == 1 {
			what = fmt.Sprintf("1 smaller item (%s) is hidden; show it", human(hiddenBytes))
		}
		fmt.Printf("\n%s %s with %s.\n", bold("Nothing over "+human(f.minSize)+" to clear."), what, bold("--min 0"))
	case safe > 0:
		fmt.Printf("\n%s. Run %s to review and clear it.\n", bold(human(safe)+" can be cleared safely"), bold("leftover clean"))
	default:
		fmt.Printf("\n%s\n", bold("Nothing to clear right now."))
	}
	if d := res.Disk; d != nil {
		line := fmt.Sprintf("%s free of %s on this disk", human(d.Free), human(d.Total))
		if after := human(d.Free + safe); after != human(d.Free) {
			line += "; clearing would make it " + after
		}
		fmt.Println(dim(line + ". " + took))
	} else {
		fmt.Println(dim(took))
	}
}

func noteMark(it scan.Item) string {
	if len(it.Notes) == 0 {
		return ""
	}
	return paint("33", "  !")
}
