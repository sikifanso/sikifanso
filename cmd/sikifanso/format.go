package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
)

// outputJSON writes data as indented JSON to stdout if --output=json is set.
// Returns true if JSON was written (caller should return), false if the caller
// should render table output. Encode errors are written to stderr.
func outputJSON(cmd *cli.Command, data any) bool {
	if cmd.String("output") != outputFormatJSON {
		return false
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: encoding JSON output: %v\n", err)
	}
	return true
}
