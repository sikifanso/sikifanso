package grpcsync

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

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
