package main

import (
	"fmt"
	"runtime"
)

// These variables are set at build time by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	fmt.Printf("sikifanso %s (commit: %s, built: %s)\n", version, commit, date)
	fmt.Printf("Go %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
