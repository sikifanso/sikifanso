package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
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

// waitSyncFlags returns the --no-wait and --timeout flags shared by all mutation commands.
func waitSyncFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "no-wait", Usage: "Trigger sync without waiting"},
		&cli.DurationFlag{Name: "timeout", Usage: "Timeout for sync wait", Value: grpcsync.DefaultTimeout},
	}
}

// grpcClientFromSession creates a gRPC client from session credentials.
func grpcClientFromSession(ctx context.Context, sess *session.Session) (*grpcclient.Client, error) {
	if sess.Services.ArgoCD.GRPCAddress == "" {
		return nil, fmt.Errorf("no gRPC address in session — cluster may need recreation")
	}
	return grpcclient.NewClient(ctx, grpcclient.Options{
		Address:  sess.Services.ArgoCD.GRPCAddress,
		Username: sess.Services.ArgoCD.Username,
		Password: sess.Services.ArgoCD.Password,
	})
}

// syncAfterMutation performs a sync after a mutation command (enable/disable/add/remove).
func syncAfterMutation(ctx context.Context, cmd *cli.Command, sess *session.Session, apps ...string) {
	client, err := grpcClientFromSession(ctx, sess)
	if err != nil {
		zapLogger.Warn("gRPC sync unavailable", zap.Error(err))
		fmt.Fprintln(os.Stderr, "ArgoCD sync unavailable — will reconcile on next interval")
		return
	}
	defer client.Close()

	orch := grpcsync.NewOrchestrator(client, zapLogger)
	req := grpcsync.Request{
		Apps:    apps,
		Timeout: cmd.Duration("timeout"),
		Prune:   true,
	}

	// If no specific apps were given, list all apps to sync.
	if len(req.Apps) == 0 {
		listed, listErr := client.ListApplications(ctx)
		if listErr != nil {
			zapLogger.Warn("listing apps for sync failed", zap.Error(listErr))
			fmt.Fprintln(os.Stderr, "ArgoCD sync: could not list apps")
			return
		}
		for _, a := range listed {
			req.Apps = append(req.Apps, a.Name)
		}
	}

	if cmd.Bool("no-wait") {
		if err := orch.SyncOnly(ctx, req); err != nil {
			zapLogger.Warn("sync trigger failed", zap.Error(err))
		}
		fmt.Fprintln(os.Stderr, "ArgoCD sync triggered")
		return
	}

	results, exitCode := orch.SyncAndWait(ctx, req)
	printSyncResults(os.Stderr, results)

	switch exitCode {
	case grpcsync.ExitFailure:
		fmt.Fprintln(os.Stderr, "sync failed: one or more apps unhealthy")
	case grpcsync.ExitTimeout:
		fmt.Fprintln(os.Stderr, "sync timed out: apps may still be reconciling")
	}
}

// printSyncResults writes a human-readable summary of sync results.
func printSyncResults(w io.Writer, results []grpcsync.Result) {
	for _, r := range results {
		indicator := "✓"
		if r.Health == "Degraded" || r.Health == "Missing" {
			indicator = "✗"
		} else if r.SyncStatus != "Synced" || r.Health != "Healthy" {
			indicator = "~"
		}

		_, _ = fmt.Fprintf(w, "  %s %s  sync=%s health=%s\n", indicator, r.App, r.SyncStatus, r.Health)

		// Print failed resources indented under the app.
		for _, res := range r.Resources {
			if res.Health != "" && res.Health != "Healthy" {
				_, _ = fmt.Fprintf(w, "      %s/%s  health=%s", res.Kind, res.Name, res.Health)
				if res.Message != "" {
					_, _ = fmt.Fprintf(w, "  %s", res.Message)
				}
				_, _ = fmt.Fprintln(w)
			}
		}
	}
}
