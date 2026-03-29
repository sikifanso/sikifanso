package grpcsync

import (
	"context"
	"sync"

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
// reach Synced+Healthy (or the timeout expires).
func (o *Orchestrator) SyncAndWait(ctx context.Context, req Request) ([]Result, ExitCode) {
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}
	// Default prune to true when not explicitly set to false by the caller.
	// Because the zero value of bool is false we rely on the caller to pass
	// Prune: true explicitly, but the spec says "default prune to true".
	req.Prune = true

	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Trigger sync for all apps first.
	for _, app := range req.Apps {
		if err := o.client.SyncApplication(ctx, app, grpcclient.SyncOptions{Prune: req.Prune}); err != nil {
			o.log.Warn("sync trigger failed", zap.String("app", app), zap.Error(err))
		}
	}

	return o.watchApps(ctx, req)
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

// watchApps watches all apps concurrently and returns their results plus a
// composite ExitCode once all goroutines have finished.
func (o *Orchestrator) watchApps(ctx context.Context, req Request) ([]Result, ExitCode) {
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
			update := func(r Result) { latest = r }
			o.watchSingleApp(ctx, appName, req.SkipUnhealthy, update)
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
		case r.SyncStatus == "Synced" && r.Health == "Healthy":
			// good — keep ExitSuccess unless already elevated
		case r.Health == "Degraded" || r.Health == "Missing":
			if code < ExitFailure {
				code = ExitFailure
			}
		default:
			// Still Progressing or unknown — treat as timeout if nothing worse.
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
func (o *Orchestrator) watchSingleApp(ctx context.Context, name string, skipUnhealthy bool, updateFn func(Result)) {
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
				// Stream closed unexpectedly — fall back to a poll.
				o.pollOnce(ctx, name, updateFn)
				return
			}

			if event.Deleted {
				updateFn(Result{
					App:        name,
					SyncStatus: "Deleted",
					Health:     "Deleted",
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

			case event.App.Health == "Degraded" && !skipUnhealthy:
				// Fetch the resource tree to enrich the result with details.
				tree, treeErr := o.client.ResourceTree(ctx, name)
				if treeErr != nil {
					o.log.Warn("resource tree fetch failed",
						zap.String("app", name), zap.Error(treeErr))
				} else {
					r.Resources = tree
				}
				updateFn(r)
				return
			}
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
