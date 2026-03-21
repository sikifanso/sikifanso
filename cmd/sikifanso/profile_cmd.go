package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/profile"
	"github.com/fatih/color"
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
		Action: func(_ context.Context, _ *cli.Command) error {
			profiles := profile.List()

			bold := color.New(color.Bold).SprintFunc()
			dim := color.New(color.FgHiBlack).SprintFunc()

			fmt.Fprintf(os.Stderr, "%s  %s  %s\n",
				bold(fmt.Sprintf("%-16s", "NAME")),
				bold(fmt.Sprintf("%-60s", "DESCRIPTION")),
				bold("APPS"),
			)

			for _, p := range profiles {
				fmt.Fprintf(os.Stderr, "%-16s  %-60s  %s\n",
					p.Name,
					p.Description,
					dim(strings.Join(p.Apps, ", ")),
				)
			}
			return nil
		},
	}
}
