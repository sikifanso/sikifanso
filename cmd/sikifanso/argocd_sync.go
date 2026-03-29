package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdSyncCmd() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Trigger ArgoCD sync for all or specific applications",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "no-wait", Usage: "Trigger sync without waiting for completion"},
			&cli.StringFlag{Name: "app", Usage: "Sync a specific application by name"},
			&cli.DurationFlag{Name: "timeout", Usage: "Timeout for sync wait", Value: grpcsync.DefaultTimeout},
			&cli.BoolFlag{Name: "skip-unhealthy", Usage: "Ignore pre-existing Degraded apps"},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return fmt.Errorf("connecting to ArgoCD gRPC: %w", err)
			}
			defer client.Close()

			orch := grpcsync.NewOrchestrator(client, zapLogger)
			req := grpcsync.Request{
				Timeout:       cmd.Duration("timeout"),
				Prune:         true,
				SkipUnhealthy: cmd.Bool("skip-unhealthy"),
			}

			if appName := cmd.String("app"); appName != "" {
				req.Apps = []string{appName}
			} else {
				// List all apps to sync.
				listed, listErr := client.ListApplications(ctx)
				if listErr != nil {
					return fmt.Errorf("listing apps for sync: %w", listErr)
				}
				for _, a := range listed {
					req.Apps = append(req.Apps, a.Name)
				}
			}

			if cmd.Bool("no-wait") {
				if err := orch.SyncOnly(ctx, req); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "ArgoCD sync triggered")
				return nil
			}

			results, exitCode := orch.SyncAndWait(ctx, req)
			printSyncResults(os.Stderr, results)

			switch exitCode {
			case grpcsync.ExitFailure:
				return fmt.Errorf("sync failed: one or more apps unhealthy")
			case grpcsync.ExitTimeout:
				return fmt.Errorf("sync timed out: apps still progressing")
			}
			return nil
		}),
	}
}
