package main

import (
	"errors"
	"fmt"
	"runtime"
	"sort"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/config"
	"github.com/MaiSamayTha/leftover/internal/rules"
)

func init() {
	register(command{name: "ignore", args: "[add|remove] <path>", short: "folders Leftover should never touch or report", run: cmdIgnore})
	register(command{name: "rules", short: "everything Leftover knows how to recognise", run: cmdRules})
	register(command{name: "version", short: "print the version", run: cmdVersion})
	register(command{name: "where", short: "show where Leftover keeps its own files", run: cmdWhere})
}

func cmdIgnore(args []string) error {
	a, err := app.Open()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		if len(a.Settings.Ignore) == 0 {
			fmt.Println("Nothing is ignored. Add a folder with: leftover ignore add <path>")
			return nil
		}
		for _, p := range a.Settings.Ignore {
			fmt.Println("  " + tilde(p))
		}
		return nil
	}
	if len(args) != 2 {
		return errors.New("usage: leftover ignore [list|add <path>|remove <path>]")
	}
	switch args[0] {
	case "add":
		if err := a.Ignore(args[1]); err != nil {
			return err
		}
		fmt.Printf("Leftover will leave %s alone from now on.\n", tilde(args[1]))
	case "remove", "rm":
		if err := a.Unignore(args[1]); err != nil {
			return err
		}
		fmt.Printf("%s will be scanned again.\n", tilde(args[1]))
	default:
		return fmt.Errorf("unknown ignore command %q", args[0])
	}
	return nil
}

func cmdRules(args []string) error {
	fs := newFlags("rules")
	asJSON := fs.Bool("json", false, "print the whole catalogue as JSON")
	here := fs.Bool("here", false, "only rules that apply to this OS")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cat := rules.MustLoad()
	host := rules.CurrentHost()
	var list []rules.Rule
	for _, r := range cat.Rules {
		if *here && r.Project == nil && len(r.Locations(host)) == 0 {
			continue
		}
		list = append(list, r)
	}
	if *asJSON {
		return printJSON(list)
	}
	groups := map[string][]rules.Rule{}
	var order []string
	for _, r := range list {
		if _, ok := groups[r.Group]; !ok {
			order = append(order, r.Group)
		}
		groups[r.Group] = append(groups[r.Group], r)
	}
	sort.Strings(order)
	for _, g := range order {
		fmt.Println(bold(g))
		for _, r := range groups[g] {
			fmt.Printf("  %s %s  %s  %s\n", verdictMark(r.Verdict), pad(r.ID, 24), pad(clip(r.Name, 32), 32), dim(r.Verdict.Label()))
		}
		fmt.Println()
	}
	fmt.Printf("%d rules. %s\n", len(list), dim("Explain one with: leftover why <path>"))
	return nil
}

func cmdVersion(args []string) error {
	fmt.Printf("leftover %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	return nil
}

func cmdWhere(args []string) error {
	dir, err := config.DataDir()
	if err != nil {
		return err
	}
	fmt.Println("Settings, history and the vault:", dir)
	fmt.Println(dim("Nothing else on your machine is written to, and nothing is ever sent anywhere."))
	return nil
}
