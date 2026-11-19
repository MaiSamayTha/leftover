package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

func init() {
	register(command{name: "docker", args: "[prune]", short: "see what Docker holds, and reclaim its cache safely", run: cmdDocker})
}

func cmdDocker(args []string) error {
	prune := len(args) > 0 && args[0] == "prune"
	if prune {
		args = args[1:]
	}
	fs := newFlags("docker")
	images := fs.Bool("images", false, "with prune: also remove every image no container uses (they're pulled again when needed)")
	yes := fs.Bool("yes", false, "with prune: don't ask")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unknown argument %q; try 'leftover docker' or 'leftover docker prune'", fs.Arg(0))
	}

	parts := scan.DockerBreakdown()
	if !prune {
		if *asJSON {
			if parts == nil {
				parts = []scan.Detail{}
			}
			return printJSON(parts)
		}
		if parts == nil {
			return errors.New("couldn't ask Docker; is it installed and running?")
		}
		fmt.Println(bold("What Docker is holding"))
		fmt.Println()
		for _, d := range parts {
			fmt.Printf("  %s  %s  %s\n", pad(d.Name, 14), lpad(human(d.Bytes), 9), dim(fmt.Sprintf("%s reclaimable  %s", human(d.Reclaimable), d.Command)))
		}
		fmt.Println()
		fmt.Println(dim("'leftover docker prune' removes the build cache and dangling images, which Docker"))
		fmt.Println(dim("re-creates on demand. Add --images for every unused image. Volumes and containers are never touched."))
		return nil
	}

	if !*asJSON {
		fmt.Println(bold("Leftover will ask Docker to run:"))
		fmt.Println()
		for _, a := range app.PrunePlan(*images) {
			fmt.Printf("  docker %s\n", strings.Join(a, " "))
		}
		fmt.Println()
		fmt.Println(dim("This is Docker's own cleanup, so it can't be undone from the vault. The build cache is"))
		fmt.Println(dim("rebuilt and images are pulled again when they're next needed. Volumes and containers stay."))
	}
	if !*yes {
		if *asJSON {
			return errors.New("--json needs --yes, since there's no one to ask")
		}
		if !askYes("Go ahead?") {
			fmt.Println("Nothing changed.")
			return nil
		}
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	ctx, stop := interruptible()
	defer stop()
	steps, err := a.DockerPrune(ctx, *images)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(steps)
	}
	var total int64
	failed := false
	for _, s := range steps {
		if s.Error != "" {
			failed = true
			fmt.Printf("  %s %s: %s\n", paint("31", "✕"), s.Command, s.Error)
			continue
		}
		total += s.Reclaimed
	}
	fmt.Printf("\n%s\n", green(fmt.Sprintf("Docker reclaimed %s.", human(total))))
	if failed {
		return errors.New("some Docker commands failed")
	}
	return nil
}
