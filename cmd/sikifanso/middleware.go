package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/appsetreconcile"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
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
// ArgoCD multiplexes gRPC and HTTP on the same port via CMux, so the
// gRPC address is derived from the HTTP URL.
func grpcClientFromSession(ctx context.Context, sess *session.Session) (*grpcclient.Client, error) {
	addr, err := grpcclient.AddressFromURL(sess.Services.ArgoCD.URL)
	if err != nil {
		return nil, err
	}
	return grpcclient.NewClient(ctx, grpcclient.Options{
		Address:  addr,
		Username: sess.Services.ArgoCD.Username,
		Password: sess.Services.ArgoCD.Password,
	})
}

// MutationOpts describes the sync behaviour after a mutation command.
type MutationOpts struct {
	Operation  grpcsync.OperationType
	Apps       []string
	AppSetName string // "catalog", "root", "agents"
}

// syncAfterMutation performs a sync after a mutation command (enable/disable/add/remove).
func syncAfterMutation(ctx context.Context, cmd *cli.Command, sess *session.Session, opts MutationOpts) error {
	// 1. Create gRPC client for app status.
	grpcClient, err := grpcClientFromSession(ctx, sess)
	if err != nil {
		zapLogger.Warn("gRPC unavailable", zap.Error(err))
		fmt.Fprintln(os.Stderr, "ArgoCD sync unavailable — will reconcile on next interval")
		return nil
	}
	defer grpcClient.Close()

	// 2. Create k8s reconciler for AppSet annotation patching.
	restCfg, err := kube.RESTConfigForCluster(sess.ClusterName)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	reconciler, err := appsetreconcile.NewReconciler(restCfg, "argocd")
	if err != nil {
		return fmt.Errorf("reconciler: %w", err)
	}

	// 3. Build request.
	orch := grpcsync.NewOrchestrator(grpcClient, zapLogger)
	req := grpcsync.Request{
		Apps:      opts.Apps,
		Timeout:   cmd.Duration("timeout"),
		Prune:     true,
		Operation: opts.Operation,
		ReconcileFn: func(ctx context.Context) error {
			return reconciler.Trigger(ctx, opts.AppSetName)
		},
		OnProgress: func(app, status, detail string) {
			fmt.Fprintf(os.Stderr, "  %s  %s", app, status)
			if detail != "" {
				fmt.Fprintf(os.Stderr, "  %s", detail)
			}
			fmt.Fprintln(os.Stderr)
		},
	}

	// 4. No-wait mode.
	if cmd.Bool("no-wait") {
		if req.ReconcileFn != nil && (opts.Operation == grpcsync.OpEnable || opts.Operation == grpcsync.OpDisable) {
			if err := req.ReconcileFn(ctx); err != nil {
				return err
			}
		}
		fmt.Fprintln(os.Stderr, "  ApplicationSet reconciliation triggered")
		return nil
	}

	// 5. Wait mode.
	results, exitCode := orch.SyncAndWait(ctx, req)
	printSyncResults(os.Stderr, results)

	switch exitCode {
	case grpcsync.ExitFailure:
		return fmt.Errorf("sync failed: one or more apps unhealthy")
	case grpcsync.ExitTimeout:
		return fmt.Errorf("sync timed out: apps may still be reconciling")
	}
	return nil
}

// printSyncResults writes a human-readable summary of sync results.
func printSyncResults(w io.Writer, results []grpcsync.Result) {
	for _, r := range results {
		indicator := "✓"
		if r.Deleted {
			indicator = "✓"
		} else if r.Health == "Degraded" || r.Health == "Missing" {
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
