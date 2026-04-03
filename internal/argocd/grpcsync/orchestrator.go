package grpcsync

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
)

// Orchestrator manages sync-and-watch operations for one or more ArgoCD applications
// using the gRPC Watch stream for real-time feedback.
type Orchestrator struct {
	client *grpcclient.Client
	log    *zap.Logger
}

// NewOrchestrator creates an Orchestrator backed by the given gRPC client.
func NewOrchestrator(client *grpcclient.Client, log *zap.Logger) *Orchestrator {
	return &Orchestrator{client: client, log: log}
}

// SyncAndWait triggers a sync for every app in req, then watches until all apps
// reach Synced+Healthy (or the timeout expires). The behaviour varies based on
// req.Operation:
//   - OpSync: trigger SyncApplication, watch for Synced+Healthy
//   - OpEnable: trigger AppSet reconciliation, wait for app to appear, then Synced+Healthy
//   - OpDisable: trigger AppSet reconciliation, wait for app deletion
func (o *Orchestrator) SyncAndWait(ctx context.Context, req Request) ([]Result, ExitCode) {
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}
	req.Prune = true

	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	switch req.Operation {
	case OpEnable:
		if req.ReconcileFn != nil {
			if err := req.ReconcileFn(ctx); err != nil {
				o.log.Warn("AppSet reconciliation failed", zap.Error(err))
			}
		}
		return o.watchApps(ctx, req, o.waitForAppear)

	case OpDisable:
		if req.ReconcileFn != nil {
			if err := req.ReconcileFn(ctx); err != nil {
				o.log.Warn("AppSet reconciliation failed", zap.Error(err))
			}
		}
		return o.watchApps(ctx, req, o.waitForDisappear)

	default: // OpSync
		for _, app := range req.Apps {
			if err := o.client.SyncApplication(ctx, app, grpcclient.SyncOptions{Prune: req.Prune}); err != nil {
				o.log.Warn("sync trigger failed", zap.String("app", app), zap.Error(err))
			}
		}
		return o.watchApps(ctx, req, o.watchSingleApp)
	}
}

// SyncOnly triggers a sync for every app in req and returns the first error encountered.
func (o *Orchestrator) SyncOnly(ctx context.Context, req Request) error {
	for _, app := range req.Apps {
		if err := o.client.SyncApplication(ctx, app, grpcclient.SyncOptions{Prune: req.Prune}); err != nil {
			return err
		}
	}
	return nil
}

// watchFn is the per-app strategy used by watchApps.
type watchFn func(ctx context.Context, name string, req Request, updateFn func(Result))

// watchApps watches all apps concurrently using the given per-app strategy and
// returns their results plus a composite ExitCode once all goroutines have finished.
func (o *Orchestrator) watchApps(ctx context.Context, req Request, fn watchFn) ([]Result, ExitCode) {
	type entry struct {
		idx int
		res Result
	}

	results := make([]Result, len(req.Apps))
	for i, name := range req.Apps {
		results[i] = Result{App: name}
	}

	ch := make(chan entry, len(req.Apps))
	var wg sync.WaitGroup

	for i, name := range req.Apps {
		wg.Add(1)
		go func(idx int, appName string) {
			defer wg.Done()
			var latest Result
			update := func(r Result) {
				latest = r
				if req.OnProgress != nil {
					detail := r.Message
					status := r.SyncStatus + "/" + r.Health
					if r.Deleted {
						status = "Deleted"
					}
					req.OnProgress(appName, status, detail)
				}
			}
			fn(ctx, appName, req, update)
			ch <- entry{idx: idx, res: latest}
		}(i, name)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for e := range ch {
		results[e.idx] = e.res
	}

	// Determine composite exit code.
	code := ExitSuccess
	for _, r := range results {
		switch {
		case r.Deleted:
			// Successful deletion — keep ExitSuccess.
		case r.SyncStatus == "Synced" && r.Health == "Healthy":
			// good
		case r.Health == "Degraded" || r.Health == "Missing":
			if code < ExitFailure {
				code = ExitFailure
			}
		default:
			if code < ExitTimeout {
				code = ExitTimeout
			}
		}
	}

	return results, code
}

// watchSingleApp opens a Watch stream for appName and calls updateFn on every
// event until the app reaches a terminal state or ctx is cancelled.
// On stream error it falls back to a single pollOnce call.
// FIX: Don't exit on Degraded immediately. Only exit on Degraded if the app is
// also Synced (meaning the sync completed but the app is broken). If the app is
// OutOfSync+Degraded, keep waiting.
func (o *Orchestrator) watchSingleApp(ctx context.Context, name string, req Request, updateFn func(Result)) {
	eventCh, err := o.client.WatchApplication(ctx, name)
	if err != nil {
		o.log.Warn("watch failed, falling back to poll", zap.String("app", name), zap.Error(err))
		o.pollOnce(ctx, name, updateFn)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-eventCh:
			if !ok {
				o.pollOnce(ctx, name, updateFn)
				return
			}

			if event.Deleted {
				updateFn(Result{
					App:        name,
					SyncStatus: "Deleted",
					Health:     "Deleted",
					Deleted:    true,
				})
				return
			}

			r := Result{
				App:        name,
				SyncStatus: event.App.SyncStatus,
				Health:     event.App.Health,
				Message:    event.App.Message,
			}
			updateFn(r)

			switch {
			case event.App.SyncStatus == "Synced" && event.App.Health == "Healthy":
				return

			case event.App.Health == "Degraded" && event.App.SyncStatus == "Synced" && !req.SkipUnhealthy:
				// Sync completed but app is broken — fetch resource details and exit.
				tree, treeErr := o.client.ResourceTree(ctx, name)
				if treeErr != nil {
					o.log.Warn("resource tree fetch failed",
						zap.String("app", name), zap.Error(treeErr))
				} else {
					r.Resources = tree
				}
				updateFn(r)
				return

				// If Degraded but still OutOfSync, keep waiting — sync hasn't finished.
			}
		}
	}
}

// waitForAppear polls for the app to exist, then watches for Synced+Healthy.
func (o *Orchestrator) waitForAppear(ctx context.Context, name string, req Request, updateFn func(Result)) {
	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	// Poll until the app exists.
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		_, err := o.client.GetApplication(ctx, name)
		if err != nil {
			if grpcclient.IsNotFound(err) {
				updateFn(Result{App: name, SyncStatus: "Waiting", Health: "Waiting", Message: "waiting for app to appear"})
				continue
			}
			o.log.Warn("GetApplication failed during waitForAppear", zap.String("app", name), zap.Error(err))
			continue
		}

		// App exists — switch to watch.
		break
	}

	// App exists — kick off a sync to avoid waiting for ArgoCD's auto-sync interval.
	if err := o.client.SyncApplication(ctx, name, grpcclient.SyncOptions{Prune: req.Prune}); err != nil {
		o.log.Debug("sync trigger after appear failed (may already be syncing)", zap.String("app", name), zap.Error(err))
	}

	// Now watch for Synced+Healthy.
	o.watchSingleApp(ctx, name, req, updateFn)
}

// waitForDisappear watches until the app is deleted or times out.
func (o *Orchestrator) waitForDisappear(ctx context.Context, name string, _ Request, updateFn func(Result)) {
	// Check if already gone.
	_, err := o.client.GetApplication(ctx, name)
	if err != nil {
		if grpcclient.IsNotFound(err) {
			updateFn(Result{App: name, SyncStatus: "Deleted", Health: "Deleted", Deleted: true})
			return
		}
		o.log.Warn("GetApplication failed during waitForDisappear", zap.String("app", name), zap.Error(err))
	}

	// Open watch stream and wait for deletion.
	eventCh, err := o.client.WatchApplication(ctx, name)
	if err != nil {
		o.log.Warn("watch failed during waitForDisappear, falling back to poll", zap.String("app", name), zap.Error(err))
		o.pollUntilGone(ctx, name, updateFn)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-eventCh:
			if !ok {
				// Stream closed — poll to check if gone.
				o.pollUntilGone(ctx, name, updateFn)
				return
			}

			if event.Deleted {
				updateFn(Result{App: name, SyncStatus: "Deleted", Health: "Deleted", Deleted: true})
				return
			}

			// Ignore all health transitions — only terminal state is Deleted.
			updateFn(Result{
				App:        name,
				SyncStatus: event.App.SyncStatus,
				Health:     event.App.Health,
				Message:    "waiting for deletion",
			})
		}
	}
}

// pollUntilGone polls GetApplication until the app is not found.
func (o *Orchestrator) pollUntilGone(ctx context.Context, name string, updateFn func(Result)) {
	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		_, err := o.client.GetApplication(ctx, name)
		if err != nil {
			if grpcclient.IsNotFound(err) {
				updateFn(Result{App: name, SyncStatus: "Deleted", Health: "Deleted", Deleted: true})
				return
			}
			o.log.Warn("poll failed during waitForDisappear", zap.String("app", name), zap.Error(err))
		}
	}
}

// pollOnce performs a single GetApplication call and passes the result to updateFn.
func (o *Orchestrator) pollOnce(ctx context.Context, name string, updateFn func(Result)) {
	detail, err := o.client.GetApplication(ctx, name)
	if err != nil {
		o.log.Warn("poll failed", zap.String("app", name), zap.Error(err))
		updateFn(Result{App: name})
		return
	}

	updateFn(Result{
		App:        name,
		SyncStatus: detail.SyncStatus,
		Health:     detail.Health,
		Message:    detail.Message,
		Resources:  detail.Resources,
	})
}
