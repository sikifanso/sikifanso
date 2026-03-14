package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func argocdSyncCmd() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Force immediate ArgoCD reconciliation",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "wait",
				Usage: "Wait for all apps to reach Synced/Healthy",
			},
			&cli.StringFlag{
				Name:  "app",
				Usage: "Sync a specific application by name",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Timeout for --wait mode",
				Value: argocd.DefaultSyncTimeout,
			},
			&cli.BoolFlag{
				Name:  "skip-unhealthy",
				Usage: "Skip syncing Degraded applications",
			},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			mode := argocd.SyncModeFire
			if cmd.String("app") != "" {
				mode = argocd.SyncModeApp
			} else if cmd.Bool("wait") {
				mode = argocd.SyncModeWait
			}

			opts := syncOptsFromSession(sess)
			opts.Mode = mode
			opts.AppName = cmd.String("app")
			opts.Timeout = cmd.Duration("timeout")
			opts.SkipUnhealthy = cmd.Bool("skip-unhealthy")

			if err := argocd.SyncWithOpts(ctx, zapLogger, opts); err != nil {
				zapLogger.Error("sync failed", zap.Error(err))
				return err
			}

			// Show status after sync — use ReportStatus (not SyncAndReport)
			// because SyncWithOpts already performed the sync.
			if mode == argocd.SyncModeWait || mode == argocd.SyncModeApp {
				argocd.ReportStatus(ctx, zapLogger, os.Stderr, opts)
			} else {
				fmt.Fprintln(os.Stderr, "ArgoCD sync triggered")
			}

			return nil
		}),
	}
}
