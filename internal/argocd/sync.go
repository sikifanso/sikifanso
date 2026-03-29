package argocd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Default polling constants for wait modes.
const (
	DefaultSyncTimeout  = 2 * time.Minute
	DefaultPollInterval = 5 * time.Second
)

// SyncMode controls how the sync command behaves.
type SyncMode int

const (
	// SyncModeFire sends webhooks and returns immediately (default).
	SyncModeFire SyncMode = iota
	// SyncModeWait sends webhooks, then polls until all apps are Synced/Healthy.
	SyncModeWait
	// SyncModeApp targets a single application by name via the REST API.
	SyncModeApp
)

// SyncOpts configures a sync operation.
type SyncOpts struct {
	Mode        SyncMode
	ClusterName string
	ArgoURL     string

	// For SyncModeWait: REST API credentials + timeouts.
	Username string
	Password string
	Timeout  time.Duration
	Interval time.Duration

	// For SyncModeApp: the application to sync.
	AppName string

	// WaitApps limits SyncModeWait to poll only these specific apps
	// instead of all apps. Used by mutation commands to wait only for
	// the app(s) they changed.
	WaitApps []string

	// SkipUnhealthy skips syncing apps that are currently Degraded.
	SkipUnhealthy bool

	// Namespace is the ArgoCD namespace. Defaults to "argocd".
	Namespace string
}

// SyncWithOpts performs a sync operation based on the provided options.
func SyncWithOpts(ctx context.Context, log *zap.Logger, opts SyncOpts) error {
	switch opts.Mode {
	case SyncModeApp:
		return syncSingleApp(ctx, log, opts)
	case SyncModeWait:
		if len(opts.WaitApps) > 0 {
			return waitForApps(ctx, log, opts)
		}
		return syncAndWait(ctx, log, opts)
	default:
		return SyncWithNamespace(ctx, log, opts.ClusterName, opts.ArgoURL, opts.Namespace)
	}
}

// Sync sends webhook push events to both the ArgoCD server (to invalidate
// the repo-server cache) and the ApplicationSet controller (to trigger
// immediate reconciliation).
func Sync(ctx context.Context, log *zap.Logger, clusterName, argocdURL string) error {
	return SyncWithNamespace(ctx, log, clusterName, argocdURL, "")
}

// SyncWithNamespace is a legacy fallback that previously sent webhook push
// events to trigger ArgoCD reconciliation. Webhook logic has been removed in
// favour of the gRPC sync path. This stub remains so that callers still
// using the REST-based fallback compile; it is a no-op.
func SyncWithNamespace(_ context.Context, log *zap.Logger, _, _, _ string) error {
	log.Debug("SyncWithNamespace is a no-op — webhook logic removed, use gRPC sync path")
	return nil
}

func syncSingleApp(ctx context.Context, log *zap.Logger, opts SyncOpts) error {
	client, err := NewClient(ctx, opts.ArgoURL, opts.Username, opts.Password)
	if err != nil {
		return fmt.Errorf("creating ArgoCD client: %w", err)
	}

	if opts.SkipUnhealthy {
		status, err := client.GetApplication(ctx, opts.AppName)
		if err != nil {
			return err
		}
		if status.Health == HealthDegraded {
			log.Warn("skipping sync — app is Degraded", zap.String("app", opts.AppName))
			return nil
		}
	}

	log.Info("syncing application", zap.String("app", opts.AppName))
	return client.SyncApplication(ctx, opts.AppName)
}

// pollState holds the common setup for poll-based wait loops.
type pollState struct {
	client  *Client
	ctx     context.Context
	cancel  context.CancelFunc
	ticker  *time.Ticker
	timeout time.Duration
}

// setupPoll fires webhooks, creates a client, and prepares the polling context.
func setupPoll(ctx context.Context, log *zap.Logger, opts SyncOpts) (*pollState, error) {
	if err := SyncWithNamespace(ctx, log, opts.ClusterName, opts.ArgoURL, opts.Namespace); err != nil {
		return nil, err
	}

	client, err := NewClient(ctx, opts.ArgoURL, opts.Username, opts.Password)
	if err != nil {
		return nil, fmt.Errorf("creating ArgoCD client for polling: %w", err)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultSyncTimeout
	}
	interval := opts.Interval
	if interval == 0 {
		interval = DefaultPollInterval
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	ticker := time.NewTicker(interval)

	return &pollState{
		client:  client,
		ctx:     ctx,
		cancel:  cancel,
		ticker:  ticker,
		timeout: timeout,
	}, nil
}

func syncAndWait(ctx context.Context, log *zap.Logger, opts SyncOpts) error {
	ps, err := setupPoll(ctx, log, opts)
	if err != nil {
		return err
	}
	defer ps.cancel()
	defer ps.ticker.Stop()

	log.Info("waiting for applications to sync", zap.Duration("timeout", ps.timeout))

	for {
		select {
		case <-ps.ctx.Done():
			return fmt.Errorf("timed out waiting for applications to sync")
		case <-ps.ticker.C:
			apps, err := ps.client.ListApplications(ps.ctx)
			if err != nil {
				log.Debug("poll failed, retrying", zap.Error(err))
				continue
			}

			allSynced := true
			healthyCount := 0
			for _, a := range apps {
				if opts.SkipUnhealthy && a.Health == HealthDegraded {
					log.Debug("skipping degraded app", zap.String("app", a.Name))
					continue
				}
				healthyCount++
				if a.SyncStatus != SyncStatusSynced || a.Health != HealthHealthy {
					allSynced = false
					break
				}
			}

			if healthyCount == 0 {
				log.Warn("all applications are degraded — nothing to wait for")
				return nil
			}

			if allSynced {
				log.Info("all applications synced and healthy")
				return nil
			}
		}
	}
}

func waitForApps(ctx context.Context, log *zap.Logger, opts SyncOpts) error {
	ps, err := setupPoll(ctx, log, opts)
	if err != nil {
		return err
	}
	defer ps.cancel()
	defer ps.ticker.Stop()

	log.Info("waiting for targeted apps",
		zap.Strings("apps", opts.WaitApps),
		zap.Duration("timeout", ps.timeout),
	)

	// remaining tracks apps we still need to reach their desired state.
	remaining := make(map[string]struct{}, len(opts.WaitApps))
	for _, name := range opts.WaitApps {
		remaining[name] = struct{}{}
	}

	// seen tracks apps that have been observed at least once.
	// If an app was seen and then returns 404, it was removed (success).
	seen := make(map[string]bool, len(opts.WaitApps))

	for {
		select {
		case <-ps.ctx.Done():
			var pending []string
			for name := range remaining {
				pending = append(pending, name)
			}
			return fmt.Errorf("timed out waiting for apps: %s", strings.Join(pending, ", "))
		case <-ps.ticker.C:
			for name := range remaining {
				status, err := ps.client.GetApplication(ps.ctx, name)
				if err != nil {
					if errors.Is(err, ErrAppNotFound) {
						if seen[name] {
							log.Info("app removed", zap.String("app", name))
							delete(remaining, name)
						} else {
							log.Debug("app not yet created", zap.String("app", name))
						}
						continue
					}
					log.Debug("poll failed for app", zap.String("app", name), zap.Error(err))
					continue
				}

				seen[name] = true

				if opts.SkipUnhealthy && status.Health == HealthDegraded {
					log.Info("skipping degraded app", zap.String("app", name))
					delete(remaining, name)
					continue
				}

				if status.SyncStatus == SyncStatusSynced && status.Health == HealthHealthy {
					log.Info("app synced and healthy", zap.String("app", name))
					delete(remaining, name)
				} else {
					log.Debug("app not ready",
						zap.String("app", name),
						zap.String("sync", status.SyncStatus),
						zap.String("health", status.Health),
					)
				}
			}

			if len(remaining) == 0 {
				log.Info("all targeted apps ready")
				return nil
			}
		}
	}
}

