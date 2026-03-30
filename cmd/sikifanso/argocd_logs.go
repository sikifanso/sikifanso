package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdLogsCmd() *cli.Command {
	return &cli.Command{
		Name:      "logs",
		Usage:     "Stream pod logs for an application",
		ArgsUsage: "APP",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "pod",
				Usage:    "Pod name to fetch logs from",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "container",
				Usage: "Container name (optional, defaults to first container)",
			},
			&cli.BoolFlag{
				Name:    "follow",
				Aliases: []string{"f"},
				Usage:   "Stream logs continuously",
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

			podName := cmd.String("pod")
			container := cmd.String("container")
			follow := cmd.Bool("follow")

			ch, err := client.PodLogs(ctx, appName, podName, container, follow)
			if err != nil {
				return fmt.Errorf("streaming logs: %w", err)
			}

			for line := range ch {
				_, _ = fmt.Fprintln(os.Stdout, line)
			}
			return nil
		}),
	}
}
