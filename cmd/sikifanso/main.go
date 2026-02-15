package main

import (
	"context"
	"os"
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
		os.Exit(1)
	}
}
