package main

import (
	"context"
	"fmt"
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
