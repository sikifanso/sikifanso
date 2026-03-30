package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdDiffCmd() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between live and desired state for an application",
		ArgsUsage: "APP",
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			appName := cmd.Args().First()
			if appName == "" {
				return fmt.Errorf("application name is required")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			resources, err := client.ManagedResources(ctx, appName)
			if err != nil {
				return fmt.Errorf("fetching managed resources for %q: %w", appName, err)
			}

			hasDiff := false
			for _, res := range resources {
				if res.LiveState == res.TargetState {
					continue
				}
				hasDiff = true
				fmt.Fprintf(os.Stderr, "--- live    %s/%s\n", res.Kind, res.Name)
				fmt.Fprintf(os.Stderr, "+++ desired %s/%s\n", res.Kind, res.Name)
				if res.Namespace != "" {
					fmt.Fprintf(os.Stderr, "    namespace: %s\n", res.Namespace)
				}
				if res.LiveState == "" {
					fmt.Fprintln(os.Stderr, "  (resource not present in live cluster)")
				} else {
					fmt.Fprintf(os.Stderr, "  live:\n%s\n", indentLines(res.LiveState, "    "))
				}
				if res.TargetState == "" {
					fmt.Fprintln(os.Stderr, "  (resource not present in desired state)")
				} else {
					fmt.Fprintf(os.Stderr, "  desired:\n%s\n", indentLines(res.TargetState, "    "))
				}
				fmt.Fprintln(os.Stderr)
			}

			if !hasDiff {
				fmt.Fprintf(os.Stderr, "No diff found for application %q — live state matches desired state.\n", appName)
			}
			return nil
		}),
	}
}

// indentLines prepends a prefix to every line of s.
func indentLines(s, prefix string) string {
	var out string
	lines := splitLines(s)
	for i, l := range lines {
		if i == 0 {
			out = prefix + l
		} else {
			out += "\n" + prefix + l
		}
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
