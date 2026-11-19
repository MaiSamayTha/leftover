package main

import (
	"errors"
	"fmt"

	"github.com/MaiSamayTha/leftover/internal/update"
)

func init() {
	register(command{name: "update", short: "check for a newer version and install it", run: cmdUpdate})
}

func cmdUpdate(args []string) error {
	fs := newFlags("update")
	checkOnly := fs.Bool("check", false, "only say whether there's a newer version")
	yes := fs.Bool("yes", false, "install without asking")
	asJSON := fs.Bool("json", false, "print the check as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := interruptible()
	defer stop()
	if !*asJSON {
		fmt.Println(dim("Asking GitHub for the latest version…"))
	}
	info, err := update.Check(ctx, version)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(info)
	}
	switch {
	case info.Method == update.Dev:
		fmt.Printf("This is a development build. The latest release is %s: %s\n", info.Latest, info.Page)
		return nil
	case !info.Newer:
		fmt.Println(green(fmt.Sprintf("You're up to date (%s).", info.Current)))
		return nil
	}
	fmt.Printf("%s %s\n", bold(fmt.Sprintf("Leftover %s is out.", info.Latest)), dim("You have "+info.Current+"."))
	fmt.Println(dim("What's new: " + info.Page))
	if info.Command != "" {
		fmt.Printf("\nUpdate it with:\n  %s\n", info.Command)
		return nil
	}
	if !info.CanApply {
		fmt.Printf("\nDownload it from %s\n", info.Page)
		return nil
	}
	if *checkOnly {
		fmt.Println("\nRun 'leftover update' to install it.")
		return nil
	}
	if !*yes && !askYes("\nInstall it now?") {
		fmt.Println("Nothing changed.")
		return nil
	}
	fmt.Println(dim("Downloading and checking the signature…"))
	applied, err := update.Apply(ctx, info)
	if err != nil {
		return err
	}
	// The Windows installer replaces the running files, so it's started
	// now and this process ends.
	if err := applied.Finish(); err != nil {
		return errors.New("couldn't start the installer: " + err.Error())
	}
	if info.Method == update.Installer {
		fmt.Println(green("The installer is updating Leftover to " + info.Latest + " in the background."))
		return nil
	}
	fmt.Println(green(applied.Message))
	return nil
}
