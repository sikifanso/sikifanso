package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/appsetreconcile"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

// rejectPositionalArgs returns an error when a command receives unexpected
// positional arguments, guiding the user toward the global --cluster flag.
func rejectPositionalArgs(cmd *cli.Command) error {
	if cmd.Args().Present() {
		return fmt.Errorf("unexpected argument %q; use --cluster/-c to specify the cluster name", cmd.Args().First())
	}
	return nil
}

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
	return grpcclient.FromSessionCreds(ctx,
		sess.Services.ArgoCD.URL,
		sess.Services.ArgoCD.Username,
		sess.Services.ArgoCD.Password,
	)
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

	// 3. Build request with spinner for progress feedback.
	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	progress := newProgressTracker(s, opts.Apps)

	orch := grpcsync.NewOrchestrator(grpcClient, zapLogger)
	req := grpcsync.Request{
		Apps:      opts.Apps,
		Timeout:   cmd.Duration("timeout"),
		Prune:     true,
		Operation: opts.Operation,
		AppTiers:  buildAppTiers(sess.GitOpsPath, opts.Apps),
		ReconcileFn: func(ctx context.Context) error {
			return reconciler.Trigger(ctx, opts.AppSetName)
		},
		OnProgress: progress.Update,
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

	// 5. Wait mode with spinner.
	s.Start()
	results, exitCode := orch.SyncAndWait(ctx, req)
	s.Stop()
	printSyncResults(os.Stderr, results)

	switch exitCode {
	case grpcsync.ExitFailure:
		if opts.Operation == grpcsync.OpDisable {
			return fmt.Errorf("disable incomplete: Application deletion still in progress (resources may have long termination grace periods — try a longer --timeout)")
		}
		return fmt.Errorf("sync failed: %s", summarizeUnhealthy(results))
	case grpcsync.ExitTimeout:
		if opts.Operation == grpcsync.OpDisable {
			return fmt.Errorf("disable timed out: Application deletion still in progress (try --timeout 10m)")
		}
		return fmt.Errorf("sync timed out: %s (try a longer --timeout)", summarizeUnhealthy(results))
	}
	return nil
}

// printSyncResults writes a human-readable summary of sync results.
func printSyncResults(w io.Writer, results []grpcsync.Result) {
	for _, r := range results {
		var indicator string
		switch {
		case r.Deleted:
			indicator = "✓"
		case r.Health == "Degraded" || r.Health == "Missing":
			indicator = "✗"
		case r.SyncStatus != "Synced" || r.Health != "Healthy":
			indicator = "~"
		default:
			indicator = "✓"
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

// summarizeUnhealthy builds a one-line summary of apps that are not Synced+Healthy,
// including the first degraded resource message per app for actionable diagnostics.
func summarizeUnhealthy(results []grpcsync.Result) string {
	var parts []string
	for _, r := range results {
		if r.Deleted || (r.SyncStatus == "Synced" && r.Health == "Healthy") {
			continue
		}
		detail := fmt.Sprintf("%s (sync=%s health=%s)", r.App, r.SyncStatus, r.Health)
		// First unhealthy resource with a non-empty message — keeps the error line scannable.
		for _, res := range r.Resources {
			if res.Health != "" && res.Health != "Healthy" && res.Message != "" {
				detail += fmt.Sprintf(" [%s/%s: %s]", res.Kind, res.Name, res.Message)
				break
			}
		}
		parts = append(parts, detail)
	}
	if len(parts) == 0 {
		return "one or more apps unhealthy"
	}
	return strings.Join(parts, "; ")
}

// progressTracker aggregates per-app status into a single spinner suffix so
// that all apps remain visible during concurrent sync (without this, only the
// last-writing goroutine's status would show).
type progressTracker struct {
	mu         sync.Mutex
	spinner    *spinner.Spinner
	apps       []string
	status     map[string]string
	lastSuffix string
}

func newProgressTracker(s *spinner.Spinner, apps []string) *progressTracker {
	return &progressTracker{
		spinner: s,
		apps:    apps,
		status:  make(map[string]string, len(apps)),
	}
}

// Update records the latest status for an app and re-renders the combined suffix.
func (p *progressTracker) Update(app, status, detail string) {
	line := fmt.Sprintf("%s %s", app, status)
	if detail != "" {
		line += "  " + detail
	}

	p.mu.Lock()
	p.status[app] = line

	var sb strings.Builder
	for _, name := range p.apps {
		if s, ok := p.status[name]; ok {
			if sb.Len() > 0 {
				sb.WriteString("  │  ")
			}
			sb.WriteString(s)
		}
	}
	suffix := " " + sb.String()
	if suffix == p.lastSuffix {
		p.mu.Unlock()
		return
	}
	p.lastSuffix = suffix
	p.mu.Unlock()

	p.spinner.Lock()
	p.spinner.Suffix = suffix
	p.spinner.Unlock()
}

// buildAppTiers returns a map of app name → tier for tier-aware sequencing.
// Returns nil when gitOpsPath is empty or catalog listing fails (falls back
// to concurrent mode).
func buildAppTiers(gitOpsPath string, apps []string) map[string]string {
	if gitOpsPath == "" {
		return nil
	}
	entries, err := catalog.List(gitOpsPath)
	if err != nil {
		zapLogger.Warn("catalog listing failed, falling back to concurrent sync", zap.Error(err))
		return nil
	}
	tierByName := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Tier != "" {
			tierByName[e.Name] = e.Tier
		}
	}
	result := make(map[string]string, len(apps))
	hasTier := false
	for _, name := range apps {
		if t, ok := tierByName[name]; ok {
			result[name] = t
			hasTier = true
		}
	}
	if !hasTier {
		return nil
	}
	return result
}
