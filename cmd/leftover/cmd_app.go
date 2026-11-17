package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/ui"
)

func init() {
	register(command{name: "app", short: "open the desktop window", run: runApp})
	openApp = runApp
}

// The window pings every few seconds; once it has been closed this long,
// Leftover exits on its own.
const windowIdle = 90 * time.Second

func runApp(args []string) error {
	fs := newFlags("app")
	noOpen := fs.Bool("no-open", false, "don't open a window, just print the address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	srv, err := ui.New(a, version)
	if err != nil {
		return err
	}
	if *noOpen {
		fmt.Println("Leftover is running at", srv.URL())
		fmt.Println(dim("Only this computer can reach it. Press Ctrl-C to stop."))
		return srv.Serve(0)
	}
	if err := ui.OpenWindow(srv.URL(), filepath.Join(a.Dir, "window")); err != nil {
		fmt.Println("Couldn't open a window. Open this address in your browser:")
		fmt.Println(srv.URL())
	}
	return srv.Serve(windowIdle)
}
