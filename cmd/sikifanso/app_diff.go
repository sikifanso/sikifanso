package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func appDiffCmd() *cli.Command {
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

func indentLines(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}
