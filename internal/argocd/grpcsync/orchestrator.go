package grpcsync

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
)

// appClient is the subset of *grpcclient.Client methods used by Orchestrator.
// Keeping it as an interface allows unit tests to inject a fake implementation.
type appClient interface {
	WatchApplication(ctx context.Context, name string) (<-chan grpcclient.WatchEvent, error)
	GetApplication(ctx context.Context, name string) (*grpcclient.AppDetail, error)
	SyncApplication(ctx context.Context, name string, opts grpcclient.SyncOptions) error
	ResourceTree(ctx context.Context, name string) ([]grpcclient.ResourceStatus, error)
}

// Orchestrator manages sync-and-watch operations for one or more ArgoCD applications
// using the gRPC Watch stream for real-time feedback.
type Orchestrator struct {
	client       appClient
	log          *zap.Logger
	pollInterval time.Duration // zero → DefaultPollInterval; set in tests for speed
}

func (o *Orchestrator) pollIntervalOrDefault() time.Duration {
	if o.pollInterval > 0 {
		return o.pollInterval
	}
	return DefaultPollInterval
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
	if req.DegradedGracePeriod == 0 {
		req.DegradedGracePeriod = DefaultDegradedGracePeriod
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

// watchApps watches apps using the given per-app strategy. When req.AppTiers
// is non-nil, apps are grouped by tier and each tier batch runs sequentially
// (tier-0 first, then tier-1, etc.). If any tier produces ExitFailure, remaining
// tiers are skipped. When AppTiers is nil, all apps run concurrently.
func (o *Orchestrator) watchApps(ctx context.Context, req Request, fn watchFn) ([]Result, ExitCode) {
	if req.AppTiers == nil {
		return o.watchAppsConcurrent(ctx, req, fn)
	}

	// Build index: app name → position in the original Apps slice.
	idxByName := make(map[string]int, len(req.Apps))
	for i, name := range req.Apps {
		idxByName[name] = i
	}

	// Group apps by tier.
	tierApps := make(map[string][]string)
	for _, name := range req.Apps {
		tier := req.AppTiers[name]
		if tier == "" {
			tier = DefaultTier
		}
		tierApps[tier] = append(tierApps[tier], name)
	}

	// Sort tiers lexically: "0-operators" < "1-data" < "2-services".
	tiers := make([]string, 0, len(tierApps))
	for t := range tierApps {
		tiers = append(tiers, t)
	}
	sort.Strings(tiers)

	results := make([]Result, len(req.Apps))
	for i, name := range req.Apps {
		results[i] = Result{App: name}
	}
	compositeCode := ExitSuccess

	for _, tier := range tiers {
		apps := tierApps[tier]
		o.log.Info("watching tier", zap.String("tier", tier), zap.Strings("apps", apps))

		// Build a sub-request with only this tier's apps (shares the same context/timeout).
		tierReq := req
		tierReq.Apps = apps

		tierResults, tierCode := o.watchAppsConcurrent(ctx, tierReq, fn)

		// Map tier results back into the overall results slice.
		for j, name := range apps {
			results[idxByName[name]] = tierResults[j]
		}

		if tierCode > compositeCode {
			compositeCode = tierCode
		}

		// Abort remaining tiers on hard failure — no point deploying services
		// if their data layer is broken.
		if tierCode == ExitFailure {
			o.log.Warn("tier failed, skipping remaining tiers", zap.String("tier", tier))
			break
		}

		// Also stop if the context expired (shared timeout pool exhausted).
		if ctx.Err() != nil {
			break
		}
	}

	return results, compositeCode
}

// watchAppsConcurrent watches all apps in req.Apps concurrently and returns
// their results plus a composite ExitCode once all goroutines have finished.
func (o *Orchestrator) watchAppsConcurrent(ctx context.Context, req Request, fn watchFn) ([]Result, ExitCode) {
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
//
// Terminal states:
//   - Synced+Healthy → success
//   - Synced+Degraded persisting for req.DegradedGracePeriod → failure (resources fetched)
//   - Deleted → deleted
//   - ctx cancelled → caller handles via ExitTimeout
//
// On stream error the function falls back to pollUntilTerminal.
func (o *Orchestrator) watchSingleApp(ctx context.Context, name string, req Request, updateFn func(Result)) {
	eventCh, err := o.client.WatchApplication(ctx, name)
	if err != nil {
		o.log.Warn("watch failed, falling back to poll", zap.String("app", name), zap.Error(err))
		o.pollUntilTerminal(ctx, name, updateFn)
		return
	}

	// graceTimer is started on the first Synced+Degraded observation.
	// A nil graceCh blocks forever in the select — effectively disabled until set.
	var (
		graceTimer *time.Timer
		graceCh    <-chan time.Time
	)
	defer func() {
		if graceTimer != nil {
			graceTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-graceCh:
			// Grace period elapsed while app remained Synced+Degraded.
			// If the overall context also expired at the same time, let ctx.Done() own
			// the result — ExitTimeout is more accurate than ExitFailure in that case.
			if ctx.Err() != nil {
				return
			}
			detail, detailErr := o.client.GetApplication(ctx, name)
			if detailErr != nil {
				o.log.Warn("GetApplication after grace period failed",
					zap.String("app", name), zap.Error(detailErr))
				// Still update so watchApps scores ExitFailure with a visible message
				// rather than using the stale zero-Resources watch-event result.
				updateFn(Result{
					App:        name,
					SyncStatus: "Synced",
					Health:     "Degraded",
					Message:    "grace period elapsed; resource detail unavailable",
				})
				return
			}
			updateFn(Result{
				App:        name,
				SyncStatus: detail.SyncStatus,
				Health:     detail.Health,
				Message:    detail.Message,
				Resources:  detail.Resources,
			})
			return

		case event, ok := <-eventCh:
			if !ok {
				o.pollUntilTerminal(ctx, name, updateFn)
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
				// App is fully healthy — success.
				return

			case event.App.Health == "Degraded" && event.App.SyncStatus == "Synced" && !req.SkipUnhealthy:
				// Sync completed but app is not yet healthy. Start the grace period
				// timer on first observation. If the app recovers before the timer
				// fires, the Synced+Healthy case above returns first.
				if graceCh == nil {
					graceTimer = time.NewTimer(req.DegradedGracePeriod)
					graceCh = graceTimer.C
				}
				// Still within grace period — keep watching.

			default:
				// App moved out of Synced+Degraded (e.g. Progressing during pod
				// restart) — cancel the timer so it doesn't fire against a state
				// that is no longer Degraded.
				if graceTimer != nil {
					graceTimer.Stop()
					graceTimer = nil
					graceCh = nil
				}
			}
		}
	}
}

// waitForAppear polls for the app to exist, then watches for Synced+Healthy.
func (o *Orchestrator) waitForAppear(ctx context.Context, name string, req Request, updateFn func(Result)) {
	ticker := time.NewTicker(o.pollIntervalOrDefault())
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
	ticker := time.NewTicker(o.pollIntervalOrDefault())
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

// pollUntilTerminal polls GetApplication in a loop until the app reaches a
// terminal state or ctx is cancelled. Used as fallback when the Watch stream
// fails or closes before the app is done.
//
// Terminal states:
//   - Synced+Healthy → success
//   - Degraded or Missing → failure
//   - ctx cancelled → caller handles via ExitTimeout
//
// The first poll happens immediately on entry (no initial delay when falling back
// from a stream error). Subsequent polls wait for the ticker first, so a cancelled
// context avoids a wasted RPC on each subsequent iteration.
func (o *Orchestrator) pollUntilTerminal(ctx context.Context, name string, updateFn func(Result)) {
	ticker := time.NewTicker(o.pollIntervalOrDefault())
	defer ticker.Stop()

	poll := func() (terminal bool) {
		detail, err := o.client.GetApplication(ctx, name)
		if err != nil {
			o.log.Warn("poll failed", zap.String("app", name), zap.Error(err))
			return false
		}
		updateFn(Result{
			App:        name,
			SyncStatus: detail.SyncStatus,
			Health:     detail.Health,
			Message:    detail.Message,
			Resources:  detail.Resources,
		})
		return (detail.SyncStatus == "Synced" && detail.Health == "Healthy") ||
			detail.Health == "Degraded" || detail.Health == "Missing"
	}

	if poll() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if poll() {
			return
		}
	}
}
