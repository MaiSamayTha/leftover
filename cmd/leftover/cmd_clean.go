package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

func init() {
	register(command{name: "clean", args: "[id...]", short: "move safe items to the vault (asks first)", run: cmdClean})
}

// A saved scan older than this is redone before cleaning, so Leftover never
// acts on a picture of the disk that has drifted.
const staleScan = time.Hour

func cmdClean(args []string) error {
	fs := newFlags("clean")
	var f filter
	var minSize string
	yes := fs.Bool("yes", false, "don't ask, just do it")
	asJSON := fs.Bool("json", false, "print the outcome as JSON (needs --yes)")
	addFilterFlags(fs, &f, &minSize)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var err error
	if f.minSize, err = parseSize(minSize); err != nil {
		return err
	}
	if *asJSON && !*yes {
		return errors.New("--json needs --yes, since there's no one to ask")
	}

	a, err := app.Open()
	if err != nil {
		return err
	}
	last := a.Last()
	if last == nil || time.Since(last.Started) > staleScan {
		if !*asJSON {
			fmt.Fprintln(os.Stderr, dim("Scanning first, so the plan matches your disk as it is now…"))
		}
		ctx, stop := interruptible()
		_, _, err := a.Scan(ctx, nil)
		stop()
		if err != nil {
			return err
		}
		last = a.Last()
	}

	var picked []scan.Item
	if ids := fs.Args(); len(ids) > 0 {
		for _, id := range ids {
			it, ok := a.Find(id)
			if !ok {
				return fmt.Errorf("%s isn't in the last scan; run leftover scan and use an id from it", id)
			}
			if !it.Cleanable() {
				return fmt.Errorf("%s (%s) is marked %q, so Leftover won't move it", id, it.Name, it.Verdict.Label())
			}
			picked = append(picked, it)
		}
	} else {
		for _, it := range last.Items {
			if it.Cleanable() && f.keep(it) && it.Bytes >= f.minSize {
				picked = append(picked, it)
			}
		}
	}
	if len(picked) == 0 {
		if *asJSON {
			return printJSON([]app.Outcome{})
		}
		fmt.Println("Nothing to clear with these filters.")
		return nil
	}

	var total int64
	for _, it := range picked {
		total += it.Bytes
	}
	if !*asJSON {
		fmt.Printf("%s\n\n", bold(fmt.Sprintf("Move %d item(s), %s, to the vault:", len(picked), human(total))))
		for _, it := range picked {
			where := it.Paths[0]
			if it.Project != "" {
				where = it.Project
			}
			fmt.Printf("  %s %s  %s  %s\n", verdictMark(it.Verdict), lpad(human(it.Bytes), 8), pad(clip(it.Name, 28), 28), clip(tilde(where), 50))
		}
		fmt.Printf("\n%s\n", dim(fmt.Sprintf("Everything can be restored for %d days with 'leftover vault restore'.", a.Settings.Keep())))
	}
	if !*yes {
		if !askYes("Move them now?") {
			fmt.Println("Nothing moved.")
			return nil
		}
	}

	ids := make([]string, len(picked))
	for i, it := range picked {
		ids[i] = it.ID
	}
	outcomes := a.Clean(ids)
	if *asJSON {
		return printJSON(outcomes)
	}
	var moved int64
	var failed int
	for _, o := range outcomes {
		if o.Error != "" {
			failed++
			fmt.Printf("  %s %s: %s\n", paint("31", "✕"), o.Name, o.Error)
			continue
		}
		moved += o.Bytes
	}
	fmt.Printf("\n%s", green(fmt.Sprintf("Moved %s to the vault.", human(moved))))
	if failed > 0 {
		fmt.Printf(" %d item(s) couldn't be moved.", failed)
	}
	fmt.Println()
	fmt.Println(dim(fmt.Sprintf("The space is freed when the vault empties, after %d days or now with 'leftover vault empty'.", a.Settings.Keep())))
	if failed > 0 {
		return errors.New("some items weren't moved")
	}
	return nil
}

// askYes asks a yes/no question on the terminal. Anything but a yes is a no,
// and with no terminal attached the answer is always no.
func askYes(q string) bool {
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		fmt.Println(dim("No terminal to ask on; pass --yes to go ahead."))
		return false
	}
	fmt.Printf("%s [y/N] ", q)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
