package main

import (
	"context"
	"os"

	"github.com/fatih/color"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	app := newApp()

	err := app.Run(context.Background(), os.Args)

	if logCleanup != nil {
		logCleanup()
	}

	if err != nil {
		_, _ = color.New(color.FgRed).Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
