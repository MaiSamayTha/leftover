package main

import (
	"fmt"
	"time"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/vault"
)

func init() {
	register(command{name: "vault", args: "[restore|empty]", short: "see, restore or empty cleaned items", run: cmdVault})
	register(command{name: "history", short: "everything Leftover has moved, restored or emptied", run: cmdHistory})
}

func cmdVault(args []string) error {
	sub := "list"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "list", "ls":
		return vaultList(args)
	case "restore":
		return vaultRestore(args)
	case "empty":
		return vaultEmpty(args)
	}
	return usageError("unknown vault command %q (use list, restore or empty)", sub)
}

func vaultList(args []string) error {
	fs := newFlags("vault")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	entries, err := a.Vault.List()
	if err != nil {
		return err
	}
	if *asJSON {
		if entries == nil {
			entries = []vault.Entry{}
		}
		return printJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Println("The vault is empty.")
		return nil
	}
	var total int64
	now := time.Now()
	for _, e := range entries {
		total += e.Bytes
		where := e.Paths[0]
		if e.Project != "" {
			where = e.Project
		}
		state := dim(until(e.Expires, now))
		if e.State == vault.Moving {
			state = paint("33", "interrupted; restore it")
		}
		fmt.Printf("  %s  %s  %s  %s  %s\n", dim(e.ID), lpad(human(e.Bytes), 8), pad(clip(e.Name, 24), 24), pad(clip(tilde(where), 40), 40), state)
	}
	fmt.Printf("\n%s held. Restore with %s, or free the space now with %s.\n",
		bold(human(total)), bold("leftover vault restore <id>")+" (or "+bold("--last")+")", bold("leftover vault empty"))
	return nil
}

func vaultRestore(args []string) error {
	fs := newFlags("vault restore")
	strict := fs.Bool("strict", false, "fail instead of restoring beside a folder that came back")
	last := fs.Bool("last", false, "restore everything from the most recent clean")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 && !*last {
		return usageError("usage: leftover vault restore <id> [id...], or --last")
	}
	if fs.NArg() > 0 && *last {
		return usageError("give ids or --last, not both")
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	ids := fs.Args()
	if *last {
		entries, err := a.Vault.List()
		if err != nil {
			return err
		}
		for _, e := range vault.LastClean(entries) {
			ids = append(ids, e.ID)
		}
		if len(ids) == 0 {
			fmt.Println("The vault is empty; there's nothing to put back.")
			return nil
		}
	}
	var failed bool
	for _, id := range ids {
		e, restored, err := a.Vault.Restore(id, *strict)
		if err != nil {
			failed = true
			fmt.Printf("  %s %s: %s\n", paint("31", "✕"), id, err)
			continue
		}
		for i, p := range restored {
			note := ""
			if i < len(e.Paths) && p != e.Paths[i] {
				note = dim("  (the original place was taken, so it's beside it)")
			}
			fmt.Printf("  %s %s%s\n", green("↩"), tilde(p), note)
		}
	}
	if failed {
		return partialError("some items weren't restored")
	}
	return nil
}

func vaultEmpty(args []string) error {
	fs := newFlags("vault empty")
	expired := fs.Bool("expired", false, "only items whose restore window has passed")
	yes := fs.Bool("yes", false, "don't ask")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	if fs.NArg() > 0 {
		if !*yes && !askYes(fmt.Sprintf("Permanently delete %d vault item(s)?", fs.NArg())) {
			return nil
		}
		var freed int64
		for _, id := range fs.Args() {
			e, err := a.Vault.Empty(id)
			if err != nil {
				return err
			}
			freed += e.Bytes
		}
		fmt.Println(green("Freed " + human(freed) + "."))
		return nil
	}
	entries, _ := a.Vault.List()
	var target []vault.Entry
	now := time.Now()
	for _, e := range entries {
		if !*expired || e.Expired(now) {
			target = append(target, e)
		}
	}
	if len(target) == 0 {
		fmt.Println("Nothing to empty.")
		return nil
	}
	var total int64
	for _, e := range target {
		total += e.Bytes
	}
	if !*yes && !askYes(fmt.Sprintf("Permanently delete %d item(s), %s? This can't be undone.", len(target), human(total))) {
		fmt.Println("Kept everything.")
		return nil
	}
	var gone []vault.Entry
	if *expired {
		gone, err = a.Vault.EmptyExpired()
	} else {
		gone, err = a.Vault.EmptyAll()
	}
	var freed int64
	for _, e := range gone {
		freed += e.Bytes
	}
	fmt.Println(green("Freed " + human(freed) + "."))
	return err
}

func cmdHistory(args []string) error {
	fs := newFlags("history")
	asJSON := fs.Bool("json", false, "print as JSON")
	limit := fs.Int("n", 30, "how many events to show (0 for all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	events, err := a.Vault.History(*limit)
	if err != nil {
		return err
	}
	if *asJSON {
		if events == nil {
			events = []vault.Event{}
		}
		return printJSON(events)
	}
	if len(events) == 0 {
		fmt.Println("Leftover hasn't changed anything on this machine yet.")
		return nil
	}
	verbs := map[string]string{"move": "Moved to vault", "restore": "Restored", "empty": "Deleted for good", "docker-prune": "Pruned by Docker"}
	for _, ev := range events {
		where := ""
		if len(ev.Paths) > 0 {
			where = tilde(ev.Paths[0])
			if len(ev.Paths) > 1 {
				where += fmt.Sprintf(" +%d more", len(ev.Paths)-1)
			}
		}
		fmt.Printf("  %s  %s  %s  %s\n", dim(ev.At.Local().Format("Jan 02 15:04")), pad(verbs[ev.Action], 16), lpad(human(ev.Bytes), 8), clip(where, 52))
	}
	return nil
}
