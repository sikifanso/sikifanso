package main

import (
	"context"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/profile"
	"github.com/urfave/cli/v3"
)

func profileCmd() *cli.Command {
	return &cli.Command{
		Name:  "profile",
		Usage: "Manage cluster profiles",
		Commands: []*cli.Command{
			profileListCmd(),
		},
	}
}

func profileListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List available cluster profiles",
		Action: func(_ context.Context, cmd *cli.Command) error {
			profiles := profile.List()
			if outputJSON(cmd, profiles) {
				return nil
			}

			headers := []string{"NAME", "DESCRIPTION", "APPS"}
			rows := make([][]string, 0, len(profiles))
			for _, p := range profiles {
				rows = append(rows, []string{p.Name, p.Description, strings.Join(p.Apps, ", ")})
			}
			printTable(os.Stderr, headers, rows)
			return nil
		},
	}
}
