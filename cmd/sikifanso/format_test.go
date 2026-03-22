package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestOutputJSON_WritesJSONToStdout(t *testing.T) {
	// Capture stdout.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	type item struct {
		Name string `json:"name"`
	}
	data := []item{{Name: "alpha"}, {Name: "bravo"}}

	var ok bool
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "output", Value: "json"},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			ok = outputJSON(c, data)
			return nil
		},
	}
	if err := cmd.Run(t.Context(), []string{"test", "--output", "json"}); err != nil {
		t.Fatal(err)
	}

	_ = w.Close()

	if !ok {
		t.Fatal("outputJSON returned false, expected true for json format")
	}

	var got []item
	if err := json.NewDecoder(r).Decode(&got); err != nil {
		t.Fatalf("decoding JSON output: %v", err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "bravo" {
		t.Errorf("unexpected JSON output: %v", got)
	}
}

func TestOutputJSON_ReturnsFalseForTable(t *testing.T) {
	var ok bool
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "output", Value: "table"},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			ok = outputJSON(c, "anything")
			return nil
		},
	}
	if err := cmd.Run(t.Context(), []string{"test"}); err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Fatal("outputJSON returned true for table format, expected false")
	}
}
