package grpcsync

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
)

func TestRequestDefaults(t *testing.T) {
	t.Parallel()
	r := Request{Timeout: DefaultTimeout}
	if r.Timeout != 5*time.Minute {
		t.Fatalf("default timeout = %v, want 5m", r.Timeout)
	}
	r.Apps = []string{"foo"}
	if len(r.Apps) != 1 || r.Apps[0] != "foo" {
		t.Fatalf("unexpected apps: %v", r.Apps)
	}
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	if ExitSuccess != 0 {
		t.Fatal("ExitSuccess must be 0")
	}
	if ExitFailure != 1 {
		t.Fatal("ExitFailure must be 1")
	}
	if ExitTimeout != 2 {
		t.Fatal("ExitTimeout must be 2")
	}
}

func TestDegradedGracePeriodDefault(t *testing.T) {
	t.Parallel()
	if DefaultDegradedGracePeriod != 60*time.Second {
		t.Fatalf("DefaultDegradedGracePeriod = %v, want 60s", DefaultDegradedGracePeriod)
	}
}

// fakeAppClient is a test double for appClient.
// events are sent to the watch channel in order, then the goroutine blocks
// until ctx is cancelled (simulating a long-lived, quiet watch stream) unless
// closeStreamAfterEvents is true, in which case the channel is closed after all
// events are sent (simulating a stream that terminates early).
//
// If watchErr is set, WatchApplication returns that error immediately.
// If detailSeq is set, GetApplication returns elements in order (repeating the
// last one once the slice is exhausted); otherwise it returns detail.
type fakeAppClient struct {
	events                 []grpcclient.WatchEvent
	detail                 *grpcclient.AppDetail
	detailSeq              []*grpcclient.AppDetail
	detailIdx              atomic.Int64
	watchErr               error
	closeStreamAfterEvents bool
}

func (f *fakeAppClient) WatchApplication(ctx context.Context, _ string) (<-chan grpcclient.WatchEvent, error) {
	if f.watchErr != nil {
		return nil, f.watchErr
	}
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
		if !f.closeStreamAfterEvents {
			// Block until ctx is cancelled — simulates a live stream with no more events.
			<-ctx.Done()
		}
		// else: let defer close(ch) run — simulates a stream that closes early.
	}()
	return ch, nil
}

func (f *fakeAppClient) GetApplication(ctx context.Context, _ string) (*grpcclient.AppDetail, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(f.detailSeq) > 0 {
		idx := int(f.detailIdx.Add(1) - 1)
		if idx >= len(f.detailSeq) {
			idx = len(f.detailSeq) - 1
		}
		d := *f.detailSeq[idx]
		return &d, nil
	}
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

// TestSyncAndWait_DegradedThenProgressing confirms that an app that transitions
// Degraded→Progressing (pod restart) resets the grace timer so the timer cannot
// fire against a non-Degraded state and produce a false ExitTimeout.
func TestSyncAndWait_DegradedThenProgressing(t *testing.T) {
	t.Parallel()
	fake := &fakeAppClient{
		events: []grpcclient.WatchEvent{
			{App: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Degraded"}},
			{App: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Progressing"}},
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
		DegradedGracePeriod: 50 * time.Millisecond, // very short to catch any timer leak
	})
	if code != ExitSuccess {
		t.Fatalf("exit code = %v (%d), want ExitSuccess; results = %+v", code, code, results)
	}
	if results[0].Health != "Healthy" {
		t.Errorf("results[0].Health = %q, want Healthy", results[0].Health)
	}
}

// TestSyncAndWait_StreamError_PollsUntilHealthy confirms that when WatchApplication
// fails (transient gRPC error), SyncAndWait falls back to pollUntilTerminal and
// waits for Synced+Healthy rather than snapshotting the first intermediate state.
func TestSyncAndWait_StreamError_PollsUntilHealthy(t *testing.T) {
	t.Parallel()
	fake := &fakeAppClient{
		watchErr: errors.New("stream unavailable"),
		detailSeq: []*grpcclient.AppDetail{
			{AppStatus: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Progressing"}},
			{AppStatus: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Progressing"}},
			{AppStatus: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Healthy"}},
		},
	}
	orch := &Orchestrator{client: fake, log: zap.NewNop(), pollInterval: time.Millisecond}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:    []string{"myapp"},
		Timeout: 10 * time.Second,
	})
	if code != ExitSuccess {
		t.Fatalf("exit code = %v (%d), want ExitSuccess; results = %+v", code, code, results)
	}
	if results[0].Health != "Healthy" {
		t.Errorf("results[0].Health = %q, want Healthy", results[0].Health)
	}
}

// TestSyncAndWait_StreamClose_PollsUntilHealthy confirms that when the Watch
// stream closes early (before the app is done), SyncAndWait continues polling
// rather than capturing the intermediate state and exiting.
func TestSyncAndWait_StreamClose_PollsUntilHealthy(t *testing.T) {
	t.Parallel()
	fake := &fakeAppClient{
		// Stream sends one Progressing event then closes early.
		events: []grpcclient.WatchEvent{
			{App: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Progressing"}},
		},
		closeStreamAfterEvents: true,
		// Poll sequence: still Progressing on first poll, then Healthy.
		detailSeq: []*grpcclient.AppDetail{
			{AppStatus: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Progressing"}},
			{AppStatus: grpcclient.AppStatus{Name: "myapp", SyncStatus: "Synced", Health: "Healthy"}},
		},
	}
	orch := &Orchestrator{client: fake, log: zap.NewNop(), pollInterval: time.Millisecond}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:    []string{"myapp"},
		Timeout: 10 * time.Second,
	})
	if code != ExitSuccess {
		t.Fatalf("exit code = %v (%d), want ExitSuccess; results = %+v", code, code, results)
	}
	if results[0].Health != "Healthy" {
		t.Errorf("results[0].Health = %q, want Healthy", results[0].Health)
	}
}

// TestSyncAndWait_DegradedPersists confirms that an app that remains Degraded
// for the full grace period is reported as ExitFailure with resource detail.
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

// --- Tier-aware watchApps tests ---

// multiAppClient is a test double that serves different watch events per app name.
// It records the order in which WatchApplication is called so tests can verify
// tier sequencing.
type multiAppClient struct {
	// perApp maps app name → events to send on its watch channel.
	perApp map[string][]grpcclient.WatchEvent
	// watchOrder records the order of WatchApplication calls (thread-safe via channel).
	watchOrder chan string
}

func newMultiAppClient(perApp map[string][]grpcclient.WatchEvent) *multiAppClient {
	return &multiAppClient{
		perApp:     perApp,
		watchOrder: make(chan string, 32),
	}
}

func (m *multiAppClient) WatchApplication(ctx context.Context, name string) (<-chan grpcclient.WatchEvent, error) {
	m.watchOrder <- name
	events := m.perApp[name]
	ch := make(chan grpcclient.WatchEvent, len(events)+1)
	go func() {
		defer close(ch)
		for _, e := range events {
			select {
			case ch <- e:
			case <-ctx.Done():
				return
			}
		}
		<-ctx.Done()
	}()
	return ch, nil
}

func (m *multiAppClient) GetApplication(_ context.Context, name string) (*grpcclient.AppDetail, error) {
	events := m.perApp[name]
	if len(events) == 0 {
		return nil, errors.New("not found")
	}
	last := events[len(events)-1]
	return &grpcclient.AppDetail{
		AppStatus: last.App,
	}, nil
}

func (m *multiAppClient) SyncApplication(_ context.Context, _ string, _ grpcclient.SyncOptions) error {
	return nil
}

func (m *multiAppClient) ResourceTree(_ context.Context, _ string) ([]grpcclient.ResourceStatus, error) {
	return nil, nil
}

// drainWatchOrder reads all entries from the watchOrder channel (non-blocking)
// and returns them in order.
func (m *multiAppClient) drainWatchOrder() []string {
	var order []string
	for {
		select {
		case name := <-m.watchOrder:
			order = append(order, name)
		default:
			return order
		}
	}
}

// healthyEvent returns a WatchEvent with Synced+Healthy for the given app name.
func healthyEvent(name string) grpcclient.WatchEvent {
	return grpcclient.WatchEvent{
		App: grpcclient.AppStatus{Name: name, SyncStatus: "Synced", Health: "Healthy"},
	}
}

// degradedEvent returns a WatchEvent with Synced+Degraded for the given app name.
func degradedEvent(name string) grpcclient.WatchEvent {
	return grpcclient.WatchEvent{
		App: grpcclient.AppStatus{Name: name, SyncStatus: "Synced", Health: "Degraded"},
	}
}

// assertInterTierOrder checks that every app in an earlier tier appears before
// every app in a later tier within the watchOrder slice. Intra-tier order is
// nondeterministic (goroutines launch concurrently within a tier).
func assertInterTierOrder(t *testing.T, order []string, tiers map[string]string) {
	t.Helper()
	indexOf := make(map[string]int, len(order))
	for i, name := range order {
		indexOf[name] = i
	}
	for a, tierA := range tiers {
		for b, tierB := range tiers {
			if tierA < tierB {
				idxA, okA := indexOf[a]
				idxB, okB := indexOf[b]
				if okA && okB && idxA >= idxB {
					t.Errorf("tier ordering violated: %s (tier %s, pos %d) should appear before %s (tier %s, pos %d)",
						a, tierA, idxA, b, tierB, idxB)
				}
			}
		}
	}
}

// TestWatchApps_TierSequencing verifies that when AppTiers is set, all apps
// in an earlier tier are watched before any app in a later tier.
// Uses multiple apps per tier to exercise intra-tier concurrency.
func TestWatchApps_TierSequencing(t *testing.T) {
	t.Parallel()

	appTiers := map[string]string{
		"cnpg-operator": "0-operators",
		"cert-manager":  "0-operators",
		"postgresql":    "1-data",
		"valkey":        "1-data",
		"langfuse":      "2-services",
		"litellm-proxy": "2-services",
	}

	perApp := make(map[string][]grpcclient.WatchEvent, len(appTiers))
	apps := make([]string, 0, len(appTiers))
	for name := range appTiers {
		perApp[name] = []grpcclient.WatchEvent{healthyEvent(name)}
		apps = append(apps, name)
	}

	fake := newMultiAppClient(perApp)
	orch := &Orchestrator{client: fake, log: zap.NewNop()}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:     apps,
		Timeout:  10 * time.Second,
		AppTiers: appTiers,
	})

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, want ExitSuccess; results = %+v", code, results)
	}

	order := fake.drainWatchOrder()
	if len(order) != len(apps) {
		t.Fatalf("watchOrder has %d entries, want %d: %v", len(order), len(apps), order)
	}

	assertInterTierOrder(t, order, appTiers)
}

// TestWatchApps_TierFailureAbortsRemaining verifies that if a tier-0 app
// fails (Degraded), tier-1 and tier-2 apps are never watched.
func TestWatchApps_TierFailureAbortsRemaining(t *testing.T) {
	t.Parallel()

	fake := newMultiAppClient(map[string][]grpcclient.WatchEvent{
		"cnpg-operator": {degradedEvent("cnpg-operator")},
		"postgresql":    {healthyEvent("postgresql")},
		"langfuse":      {healthyEvent("langfuse")},
	})

	orch := &Orchestrator{client: fake, log: zap.NewNop()}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:    []string{"langfuse", "cnpg-operator", "postgresql"},
		Timeout: 10 * time.Second,
		AppTiers: map[string]string{
			"cnpg-operator": "0-operators",
			"postgresql":    "1-data",
			"langfuse":      "2-services",
		},
		DegradedGracePeriod: 50 * time.Millisecond, // short for test speed
	})

	if code != ExitFailure {
		t.Fatalf("exit code = %d, want ExitFailure; results = %+v", code, results)
	}

	order := fake.drainWatchOrder()
	// Only tier-0 should have been watched — tier-1 and tier-2 should be skipped.
	if len(order) != 1 {
		t.Fatalf("watchOrder has %d entries, want 1 (only tier-0): %v", len(order), order)
	}
	if order[0] != "cnpg-operator" {
		t.Errorf("watched app = %q, want cnpg-operator", order[0])
	}

	// Verify that skipped apps still have their initial empty results.
	for _, r := range results {
		if r.App == "langfuse" && r.Health == "Healthy" {
			t.Error("langfuse should not have been watched")
		}
		if r.App == "postgresql" && r.Health == "Healthy" {
			t.Error("postgresql should not have been watched")
		}
	}
}

// TestWatchApps_NilAppTiers_Concurrent verifies backward compatibility:
// when AppTiers is nil, all apps are watched concurrently.
func TestWatchApps_NilAppTiers_Concurrent(t *testing.T) {
	t.Parallel()

	fake := newMultiAppClient(map[string][]grpcclient.WatchEvent{
		"app-a": {healthyEvent("app-a")},
		"app-b": {healthyEvent("app-b")},
		"app-c": {healthyEvent("app-c")},
	})

	orch := &Orchestrator{client: fake, log: zap.NewNop()}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:    []string{"app-a", "app-b", "app-c"},
		Timeout: 10 * time.Second,
		// AppTiers is nil — concurrent mode.
	})

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, want ExitSuccess; results = %+v", code, results)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for _, r := range results {
		if r.Health != "Healthy" {
			t.Errorf("app %s health = %q, want Healthy", r.App, r.Health)
		}
	}
}

// TestWatchApps_DefaultTier verifies that apps missing from AppTiers map
// are placed into the default tier (0-operators).
func TestWatchApps_DefaultTier(t *testing.T) {
	t.Parallel()

	fake := newMultiAppClient(map[string][]grpcclient.WatchEvent{
		"untiered-app": {healthyEvent("untiered-app")},
		"langfuse":     {healthyEvent("langfuse")},
	})

	orch := &Orchestrator{client: fake, log: zap.NewNop()}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:    []string{"langfuse", "untiered-app"},
		Timeout: 10 * time.Second,
		AppTiers: map[string]string{
			// untiered-app is not in the map — should default to "0-operators"
			"langfuse": "2-services",
		},
	})

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, want ExitSuccess; results = %+v", code, results)
	}

	order := fake.drainWatchOrder()
	if len(order) != 2 {
		t.Fatalf("watchOrder has %d entries, want 2: %v", len(order), order)
	}
	// untiered-app defaults to "0-operators", langfuse is "2-services".
	assertInterTierOrder(t, order, map[string]string{
		"untiered-app": DefaultTier,
		"langfuse":     "2-services",
	})
}

// TestWatchApps_DisableReversesOrder verifies that OpDisable reverses tier
// ordering so services are torn down before their dependencies.
func TestWatchApps_DisableReversesOrder(t *testing.T) {
	t.Parallel()

	deletedClient := &alreadyDeletedClient{watchOrder: make(chan string, 32)}

	orch := &Orchestrator{client: deletedClient, log: zap.NewNop()}
	appTiers := map[string]string{
		"cnpg-operator": "0-operators",
		"postgresql":    "1-data",
		"langfuse":      "2-services",
	}
	results, code := orch.SyncAndWait(context.Background(), Request{
		Apps:      []string{"cnpg-operator", "postgresql", "langfuse"},
		Timeout:   10 * time.Second,
		Operation: OpDisable,
		AppTiers:  appTiers,
	})

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, want ExitSuccess; results = %+v", code, results)
	}

	order := deletedClient.drainWatchOrder()
	if len(order) != 3 {
		t.Fatalf("watchOrder has %d entries, want 3: %v", len(order), order)
	}

	// For disable, tier order is reversed: 2-services before 1-data before 0-operators.
	// Use reverse tier keys for the inter-tier assertion.
	reverseTiers := map[string]string{
		"langfuse":      "0-first",  // 2-services should be watched first
		"postgresql":    "1-second", // 1-data second
		"cnpg-operator": "2-third",  // 0-operators last
	}
	assertInterTierOrder(t, order, reverseTiers)
}

// alreadyDeletedClient reports all apps as not found (already deleted),
// so waitForDisappear returns immediately. Tracks watch order for sequencing tests.
type alreadyDeletedClient struct {
	watchOrder chan string
}

func (a *alreadyDeletedClient) WatchApplication(_ context.Context, _ string) (<-chan grpcclient.WatchEvent, error) {
	return nil, errors.New("not found")
}

func (a *alreadyDeletedClient) GetApplication(_ context.Context, name string) (*grpcclient.AppDetail, error) {
	a.watchOrder <- name
	return nil, status.Error(codes.NotFound, "app not found")
}

func (a *alreadyDeletedClient) SyncApplication(_ context.Context, _ string, _ grpcclient.SyncOptions) error {
	return nil
}

func (a *alreadyDeletedClient) ResourceTree(_ context.Context, _ string) ([]grpcclient.ResourceStatus, error) {
	return nil, nil
}

func (a *alreadyDeletedClient) drainWatchOrder() []string {
	var order []string
	for {
		select {
		case name := <-a.watchOrder:
			order = append(order, name)
		default:
			return order
		}
	}
}
