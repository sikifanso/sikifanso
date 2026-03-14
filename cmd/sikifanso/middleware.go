package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

// wrapAction adds timing and structured logging to any command action.
func wrapAction(action cli.ActionFunc) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		start := time.Now()
		err := action(ctx, cmd)
		zapLogger.Info("command finished",
			zap.String("cmd", cmd.FullName()),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err),
		)
		return err
	}
}

// withSession loads the cluster session and passes it to the handler,
// eliminating the repeated session-loading boilerplate across commands.
func withSession(fn func(ctx context.Context, cmd *cli.Command, sess *session.Session) error) cli.ActionFunc {
	return wrapAction(func(ctx context.Context, cmd *cli.Command) error {
		clusterName := cmd.String("cluster")
		sess, err := session.Load(clusterName)
		if err != nil {
			return fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
		}
		return fn(ctx, cmd, sess)
	})
}

// syncOptsFromSession builds SyncOpts from a session, reducing the 5-line
// struct literal that was previously repeated at every call site.
func syncOptsFromSession(sess *session.Session) argocd.SyncOpts {
	return argocd.SyncOpts{
		ClusterName: sess.ClusterName,
		ArgoURL:     sess.Services.ArgoCD.URL,
		Username:    sess.Services.ArgoCD.Username,
		Password:    sess.Services.ArgoCD.Password,
	}
}

// waitSyncFlags returns the --wait and --timeout flags shared by all mutation commands.
func waitSyncFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "wait", Usage: "Wait for apps to reach Synced/Healthy after sync"},
		&cli.DurationFlag{Name: "timeout", Usage: "Timeout for --wait", Value: argocd.DefaultSyncTimeout},
	}
}

// syncAfterMutation performs a sync after a mutation command (enable/disable/add/remove).
// When --wait is set, it uses SyncModeWait to block until apps are Synced/Healthy.
// If app names are provided, only those specific apps are polled instead of all apps.
func syncAfterMutation(ctx context.Context, cmd *cli.Command, sess *session.Session, apps ...string) {
	opts := syncOptsFromSession(sess)
	if cmd.Bool("wait") {
		opts.Mode = argocd.SyncModeWait
		opts.Timeout = cmd.Duration("timeout")
		if len(apps) > 0 {
			opts.WaitApps = apps
		}
		if err := argocd.SyncWithOpts(ctx, zapLogger, opts); err != nil {
			zapLogger.Warn("sync wait failed", zap.Error(err))
			fmt.Fprintf(os.Stderr, "sync timed out — apps may still be reconciling\n")
		}
		argocd.ReportStatus(ctx, zapLogger, os.Stderr, opts)
		return
	}
	argocd.SyncAndReport(ctx, zapLogger, os.Stderr, opts)
}
