package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdRollbackCmd() *cli.Command {
	return &cli.Command{
		Name:      "rollback",
		Usage:     "Roll back an application to a previous revision",
		ArgsUsage: "APP",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "revision",
				Usage: "History revision ID to rollback to (0 = previous)",
				Value: 0,
			},
		},
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

			revisionID := int64(cmd.Int("revision"))
			if err := client.Rollback(ctx, appName, revisionID); err != nil {
				return fmt.Errorf("rolling back %q: %w", appName, err)
			}

			fmt.Fprintf(os.Stderr, "Application %q rolled back to revision %d.\n", appName, revisionID)
			return nil
		}),
	}
}
