# Degraded Grace Period Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent `watchSingleApp` from reporting false failures when ArgoCD emits a transient `Synced+Degraded` state during normal chart startup (CRD establishment, PVC binding, image pull retries).

**Architecture:** Add a `DegradedGracePeriod` field to `Request`. When `watchSingleApp` first observes `Synced+Degraded`, it starts a `time.Timer` instead of exiting immediately. If the app transitions to `Synced+Healthy` before the timer fires, the result is success. If the timer fires while still Degraded, the orchestrator fetches the final state via `GetApplication` and exits with `ExitFailure`. Extract an `appClient` interface so the orchestrator can be unit-tested without a real gRPC connection.

**Tech Stack:** Go 1.25+, `go.uber.org/zap`, `time.Timer`, package `internal/argocd/grpcsync`

---

## Files

| File | Change |
|------|--------|
| `internal/argocd/grpcsync/orchestrator.go` | Add `appClient` interface; change `Orchestrator.client` field type; rewrite `watchSingleApp` grace-period logic |
| `internal/argocd/grpcsync/types.go` | Add `DefaultDegradedGracePeriod` constant; add `DegradedGracePeriod time.Duration` to `Request` |
| `internal/argocd/grpcsync/orchestrator_test.go` | Add `fakeAppClient`; add two new test cases |

No other files change. `NewOrchestrator` keeps its existing `*grpcclient.Client` parameter — it satisfies `appClient` implicitly.

---

## Task 1: Extract `appClient` interface and update `Orchestrator` struct

**Files:**
- Modify: `internal/argocd/grpcsync/orchestrator.go:15-24` (imports + struct)

The `Orchestrator.client` field is currently `*grpcclient.Client`. Changing it to the interface below makes the orchestrator testable without a gRPC connection. `*grpcclient.Client` already has all four methods so no call site changes.

- [ ] **Step 1: Verify existing tests pass before touching anything**

```bash
go test ./internal/argocd/grpcsync/... -race -count=1
```

Expected output:
```
ok  	github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync	Xs
```

- [ ] **Step 2: Add the `appClient` interface at the top of `orchestrator.go`, just before the `Orchestrator` struct**

Replace the imports block and struct definition (lines 1–23) with:

```go
package grpcsync

import (
	"context"
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
	client appClient
	log    *zap.Logger
}

// NewOrchestrator creates an Orchestrator backed by the given gRPC client.
func NewOrchestrator(client *grpcclient.Client, log *zap.Logger) *Orchestrator {
	return &Orchestrator{client: client, log: log}
}
```

- [ ] **Step 3: Run tests to confirm no regression**

```bash
go test ./internal/argocd/grpcsync/... -race -count=1
```

Expected:
```
ok  	github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync	Xs
```

- [ ] **Step 4: Run full test suite to confirm nothing else broke**

```bash
make test 2>&1 | grep -E "FAIL|ok"
```

Expected: all `ok`, no `FAIL`.

- [ ] **Step 5: Commit**

```bash
git add internal/argocd/grpcsync/orchestrator.go
git commit -m "grpcsync: extract appClient interface for testability"
```

---

## Task 2: Add `DefaultDegradedGracePeriod` and `DegradedGracePeriod` to `Request`

**Files:**
- Modify: `internal/argocd/grpcsync/types.go`
- Modify: `internal/argocd/grpcsync/orchestrator.go:31-35` (the `SyncAndWait` defaults block)
- Modify: `internal/argocd/grpcsync/orchestrator_test.go`

- [ ] **Step 1: Write the failing test in `orchestrator_test.go`**

Add this test to the existing file:

```go
func TestDegradedGracePeriodDefault(t *testing.T) {
	t.Parallel()
	// SyncAndWait should apply DefaultDegradedGracePeriod when the field is zero.
	if DefaultDegradedGracePeriod != 60*time.Second {
		t.Fatalf("DefaultDegradedGracePeriod = %v, want 60s", DefaultDegradedGracePeriod)
	}
	req := Request{Timeout: DefaultTimeout}
	// Simulate the defaults block in SyncAndWait.
	if req.DegradedGracePeriod == 0 {
		req.DegradedGracePeriod = DefaultDegradedGracePeriod
	}
	if req.DegradedGracePeriod != 60*time.Second {
		t.Fatalf("DegradedGracePeriod after default = %v, want 60s", req.DegradedGracePeriod)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/argocd/grpcsync/... -run TestDegradedGracePeriodDefault -v
```

Expected: `FAIL — DefaultDegradedGracePeriod undefined`

- [ ] **Step 3: Add the constant and field**

In `internal/argocd/grpcsync/types.go`, update the constants block and `Request` struct:

```go
package grpcsync

import (
	"context"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
)

const (
	DefaultTimeout             = 5 * time.Minute
	DefaultPollInterval        = 5 * time.Second
	DefaultDegradedGracePeriod = 60 * time.Second
)

// OperationType indicates what kind of sync operation to perform.
type OperationType int

const (
	OpSync    OperationType = iota // existing app: SyncApplication -> wait Synced+Healthy
	OpEnable                       // app appears: reconcile AppSet -> wait for app to exist -> Synced+Healthy
	OpDisable                      // app disappears: reconcile AppSet -> wait for app deletion
)

// ProgressFn is called with status updates during sync operations.
type ProgressFn func(app string, status string, detail string)

// Request describes a sync-and-wait operation for one or more ArgoCD applications.
type Request struct {
	Apps                []string
	Timeout             time.Duration
	Prune               bool
	SkipUnhealthy       bool
	Operation           OperationType
	OnProgress          ProgressFn
	ReconcileFn         func(ctx context.Context) error
	// DegradedGracePeriod is the maximum time watchSingleApp waits after first
	// observing Synced+Degraded before declaring the app failed. A zero value is
	// replaced by DefaultDegradedGracePeriod in SyncAndWait.
	DegradedGracePeriod time.Duration
}

// Result holds the observed state of a single application after a sync operation.
type Result struct {
	App        string
	SyncStatus string
	Health     string
	Message    string
	Resources  []grpcclient.ResourceStatus
	Deleted    bool
}

// ExitCode indicates the overall outcome of a SyncAndWait call.
type ExitCode int

const (
	ExitSuccess ExitCode = 0
	ExitFailure ExitCode = 1
	ExitTimeout ExitCode = 2
)
```

- [ ] **Step 4: Apply the default in `SyncAndWait`**

In `orchestrator.go`, update the defaults block at the top of `SyncAndWait` (currently lines 31–35):

```go
func (o *Orchestrator) SyncAndWait(ctx context.Context, req Request) ([]Result, ExitCode) {
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}
	if req.DegradedGracePeriod == 0 {
		req.DegradedGracePeriod = DefaultDegradedGracePeriod
	}
	req.Prune = true
	// ... rest of function unchanged
```

- [ ] **Step 5: Run the new test to confirm it passes**

```bash
go test ./internal/argocd/grpcsync/... -run TestDegradedGracePeriodDefault -v
```

Expected:
```
--- PASS: TestDegradedGracePeriodDefault
PASS
```

- [ ] **Step 6: Run full grpcsync tests**

```bash
go test ./internal/argocd/grpcsync/... -race -count=1 -v
```

Expected: all tests pass, no race.

- [ ] **Step 7: Commit**

```bash
git add internal/argocd/grpcsync/types.go internal/argocd/grpcsync/orchestrator.go internal/argocd/grpcsync/orchestrator_test.go
git commit -m "grpcsync: add DegradedGracePeriod to Request (default 60s)"
```

---

## Task 3: Implement the grace-period timer in `watchSingleApp`

**Files:**
- Modify: `internal/argocd/grpcsync/orchestrator.go` — `watchSingleApp` function (lines 154–211)
- Modify: `internal/argocd/grpcsync/orchestrator_test.go` — add `fakeAppClient` and two test cases

- [ ] **Step 1: Write the failing tests**

Add the following to `internal/argocd/grpcsync/orchestrator_test.go`:

```go
import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"go.uber.org/zap"
)

// fakeAppClient is a test double for appClient.
// events are sent to the watch channel in order, then the goroutine blocks
// until ctx is cancelled (simulating a long-lived, quiet watch stream).
type fakeAppClient struct {
	events []grpcclient.WatchEvent
	detail *grpcclient.AppDetail
}

func (f *fakeAppClient) WatchApplication(ctx context.Context, _ string) (<-chan grpcclient.WatchEvent, error) {
	ch := make(chan grpcclient.WatchEvent, 16)
	go func() {
		defer close(ch)
		for _, e := range f.events {
			select {
			case ch <- e:
			case <-ctx.Done():
				return
			}
		}
		// Block until ctx is cancelled — simulates a live stream with no more events.
		<-ctx.Done()
	}()
	return ch, nil
}

func (f *fakeAppClient) GetApplication(_ context.Context, _ string) (*grpcclient.AppDetail, error) {
	if f.detail == nil {
		return nil, errors.New("not found")
	}
	d := *f.detail
	return &d, nil
}

func (f *fakeAppClient) SyncApplication(_ context.Context, _ string, _ grpcclient.SyncOptions) error {
	return nil
}

func (f *fakeAppClient) ResourceTree(_ context.Context, _ string) ([]grpcclient.ResourceStatus, error) {
	if f.detail == nil {
		return nil, nil
	}
	return f.detail.Resources, nil
}

// TestSyncAndWait_DegradedThenHealthy confirms that an app that starts Degraded
// but recovers to Healthy within the grace period is reported as success.
func TestSyncAndWait_DegradedThenHealthy(t *testing.T) {
	t.Parallel()
	fake := &fakeAppClient{
		events: []grpcclient.WatchEvent{
			{App: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Degraded", Message: "CRD is not established"}},
			{App: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Healthy"}},
		},
		detail: &grpcclient.AppDetail{
			AppStatus: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Healthy"},
		},
	}
	orch := &Orchestrator{client: fake, log: zap.NewNop()}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:                []string{"myapp"},
		Timeout:             10 * time.Second,
		DegradedGracePeriod: 5 * time.Second,
	})
	if code != ExitSuccess {
		t.Fatalf("exit code = %v (%d), want ExitSuccess; results = %+v", code, code, results)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Health != "Healthy" {
		t.Errorf("results[0].Health = %q, want Healthy", results[0].Health)
	}
}

// TestSyncAndWait_DegradedPersists confirms that an app that remains Degraded
// for the full grace period is reported as ExitFailure with the resource detail.
func TestSyncAndWait_DegradedPersists(t *testing.T) {
	t.Parallel()
	degradedResources := []grpcclient.ResourceStatus{
		{Kind: "CustomResourceDefinition", Name: "prometheuses.monitoring.coreos.com", Health: "Degraded", Message: "CRD is not established"},
	}
	fake := &fakeAppClient{
		events: []grpcclient.WatchEvent{
			{App: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Degraded", Message: "CRD is not established"}},
			// No further events — channel blocks until ctx is cancelled.
		},
		detail: &grpcclient.AppDetail{
			AppStatus: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Degraded"},
			Resources: degradedResources,
		},
	}
	orch := &Orchestrator{client: fake, log: zap.NewNop()}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:                []string{"myapp"},
		Timeout:             10 * time.Second,
		DegradedGracePeriod: 100 * time.Millisecond, // short for test speed
	})
	if code != ExitFailure {
		t.Fatalf("exit code = %v (%d), want ExitFailure; results = %+v", code, code, results)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Health != "Degraded" {
		t.Errorf("results[0].Health = %q, want Degraded", results[0].Health)
	}
	if len(results[0].Resources) == 0 {
		t.Error("results[0].Resources is empty — expected resource detail after grace period")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/argocd/grpcsync/... -run "TestSyncAndWait_" -v -timeout 30s
```

Expected: both tests fail because the grace-period logic is not yet implemented. `TestSyncAndWait_DegradedThenHealthy` fails with `ExitFailure` (exits immediately on first Degraded). `TestSyncAndWait_DegradedPersists` may time out or hang.

- [ ] **Step 3: Rewrite `watchSingleApp` with the grace-period timer**

Replace the entire `watchSingleApp` function in `orchestrator.go` (currently lines 154–211):

```go
// watchSingleApp opens a Watch stream for appName and calls updateFn on every
// event until the app reaches a terminal state or ctx is cancelled.
//
// Terminal states:
//   - Synced+Healthy → success
//   - Synced+Degraded persisting for req.DegradedGracePeriod → failure (resources fetched)
//   - Deleted → deleted
//   - ctx cancelled → caller handles via ExitTimeout
//
// On stream error the function falls back to a single pollOnce call.
func (o *Orchestrator) watchSingleApp(ctx context.Context, name string, req Request, updateFn func(Result)) {
	eventCh, err := o.client.WatchApplication(ctx, name)
	if err != nil {
		o.log.Warn("watch failed, falling back to poll", zap.String("app", name), zap.Error(err))
		o.pollOnce(ctx, name, updateFn)
		return
	}

	// graceTimer is started on the first Synced+Degraded observation.
	// A nil graceCh blocks forever in the select — effectively disabled.
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
			// Fetch the current state (includes per-resource detail) and exit as failure.
			detail, detailErr := o.client.GetApplication(ctx, name)
			if detailErr != nil {
				o.log.Warn("GetApplication after grace period failed",
					zap.String("app", name), zap.Error(detailErr))
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
			}
		}
	}
}
```

- [ ] **Step 4: Run the new tests**

```bash
go test ./internal/argocd/grpcsync/... -run "TestSyncAndWait_" -v -timeout 30s
```

Expected:
```
--- PASS: TestSyncAndWait_DegradedThenHealthy
--- PASS: TestSyncAndWait_DegradedPersists
PASS
```

`TestSyncAndWait_DegradedPersists` should complete in ~100ms (the short grace period in the test).

- [ ] **Step 5: Run the full grpcsync test suite with the race detector**

```bash
go test ./internal/argocd/grpcsync/... -race -count=1 -v
```

Expected: all tests pass, no race conditions reported.

- [ ] **Step 6: Run the full test suite**

```bash
make test 2>&1 | grep -E "FAIL|ok"
```

Expected: all `ok`, no `FAIL`.

- [ ] **Step 7: Commit**

```bash
git add internal/argocd/grpcsync/orchestrator.go internal/argocd/grpcsync/orchestrator_test.go
git commit -m "grpcsync: add Degraded grace period to watchSingleApp

The confirmed cause of 'sync failed: one or more apps unhealthy' on charts that
install CRDs (e.g. prometheus-stack): ArgoCD emits Synced+Degraded for 5-30s
while CRDs establish, PVCs bind, and images pull. The orchestrator was exiting
on the first such event.

watchSingleApp now starts a grace-period timer (default 60s, req.DegradedGracePeriod)
on first Synced+Degraded. If the app recovers to Synced+Healthy before the timer
fires, the result is ExitSuccess. Only if the timer expires while still Degraded
is ExitFailure returned, with per-resource detail from GetApplication.

Fixes T1 from the deployment research report."
```

---

## Self-Review

**Spec coverage:**
- [x] P1 / T1 from research report: `watchSingleApp` no longer exits on first `Synced+Degraded`
- [x] `DegradedGracePeriod` field in `Request` with `DefaultDegradedGracePeriod = 60s`
- [x] Default applied in `SyncAndWait` (zero value → 60s)
- [x] Test: `Synced+Degraded → Synced+Healthy` within grace → `ExitSuccess`
- [x] Test: `Synced+Degraded` persists past grace → `ExitFailure` with resources
- [x] `appClient` interface for testability

**Placeholder scan:** No TBD, no TODO, all code blocks complete.

**Type consistency:**
- `appClient` interface defined in Task 1, used in Task 3 via `&Orchestrator{client: fake}`
- `fakeAppClient` implements all four `appClient` methods
- `Request.DegradedGracePeriod` defined in Task 2, used in Task 3
- `graceCh <-chan time.Time` is nil until first Degraded — correct select semantics
