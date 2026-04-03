package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

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

// printTable writes a tabwriter-formatted table to w.
func printTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}
