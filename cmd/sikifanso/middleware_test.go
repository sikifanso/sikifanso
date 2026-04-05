package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/briandowns/spinner"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
)

func TestSummarizeUnhealthy(t *testing.T) {
	tests := []struct {
		name    string
		results []grpcsync.Result
		want    string
	}{
		{
			name:    "all healthy returns fallback",
			results: []grpcsync.Result{{App: "a", SyncStatus: "Synced", Health: "Healthy"}},
			want:    "one or more apps unhealthy",
		},
		{
			name:    "empty results returns fallback",
			results: nil,
			want:    "one or more apps unhealthy",
		},
		{
			name: "single degraded app",
			results: []grpcsync.Result{
				{App: "langfuse", SyncStatus: "Synced", Health: "Degraded"},
			},
			want: "langfuse (sync=Synced health=Degraded)",
		},
		{
			name: "degraded app with resource message",
			results: []grpcsync.Result{
				{
					App: "langfuse", SyncStatus: "Synced", Health: "Degraded",
					Resources: []grpcclient.ResourceStatus{
						{Kind: "Deployment", Name: "langfuse-web", Health: "Degraded", Message: "CrashLoopBackOff"},
					},
				},
			},
			want: "langfuse (sync=Synced health=Degraded) [Deployment/langfuse-web: CrashLoopBackOff]",
		},
		{
			name: "multiple unhealthy apps",
			results: []grpcsync.Result{
				{App: "valkey", SyncStatus: "Synced", Health: "Healthy"},
				{App: "langfuse", SyncStatus: "OutOfSync", Health: "Progressing"},
				{App: "presidio", SyncStatus: "Synced", Health: "Degraded"},
			},
			want: "langfuse (sync=OutOfSync health=Progressing); presidio (sync=Synced health=Degraded)",
		},
		{
			name: "deleted apps excluded",
			results: []grpcsync.Result{
				{App: "langfuse", Deleted: true},
				{App: "presidio", SyncStatus: "Synced", Health: "Missing"},
			},
			want: "presidio (sync=Synced health=Missing)",
		},
		{
			name: "only first degraded resource included",
			results: []grpcsync.Result{
				{
					App: "langfuse", SyncStatus: "Synced", Health: "Degraded",
					Resources: []grpcclient.ResourceStatus{
						{Kind: "Deployment", Name: "web", Health: "Degraded", Message: "CrashLoopBackOff"},
						{Kind: "Deployment", Name: "worker", Health: "Degraded", Message: "OOMKilled"},
					},
				},
			},
			want: "langfuse (sync=Synced health=Degraded) [Deployment/web: CrashLoopBackOff]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeUnhealthy(tt.results)
			if got != tt.want {
				t.Errorf("summarizeUnhealthy() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestProgressTracker_SingleApp(t *testing.T) {
	t.Parallel()
	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	pt := newProgressTracker(s, []string{"langfuse"})

	pt.Update("langfuse", "Synced/Healthy", "")

	s.Lock()
	got := s.Suffix
	s.Unlock()

	want := " langfuse Synced/Healthy"
	if got != want {
		t.Errorf("suffix = %q, want %q", got, want)
	}
}

func TestProgressTracker_MultiApp_PreservesOrder(t *testing.T) {
	t.Parallel()
	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	apps := []string{"valkey", "langfuse", "presidio"}
	pt := newProgressTracker(s, apps)

	// Update in reverse order — suffix should still match the original app order.
	pt.Update("presidio", "Synced/Healthy", "")
	pt.Update("valkey", "OutOfSync/Progressing", "syncing")
	pt.Update("langfuse", "Synced/Degraded", "CrashLoopBackOff")

	s.Lock()
	got := s.Suffix
	s.Unlock()

	want := " valkey OutOfSync/Progressing  syncing  │  langfuse Synced/Degraded  CrashLoopBackOff  │  presidio Synced/Healthy"
	if got != want {
		t.Errorf("suffix =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestProgressTracker_ConcurrentUpdates(t *testing.T) {
	t.Parallel()
	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Start()
	defer s.Stop()

	apps := []string{"a", "b", "c", "d", "e"}
	pt := newProgressTracker(s, apps)
	const iterations = 50

	var wg sync.WaitGroup
	for _, app := range apps {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				pt.Update(name, "Progressing", fmt.Sprintf("wave-%d", i))
			}
		}(app)
	}
	wg.Wait()

	// After all goroutines finish, the suffix must reflect the final iteration
	// for every app. Using iteration-specific strings ensures a stale suffix
	// from an earlier iteration would be caught.
	s.Lock()
	got := s.Suffix
	s.Unlock()
	lastDetail := fmt.Sprintf("wave-%d", iterations-1)
	for _, app := range apps {
		want := fmt.Sprintf("%s Progressing  %s", app, lastDetail)
		if !strings.Contains(got, want) {
			t.Errorf("suffix missing final state for app %q\n  got:  %q\n  want fragment: %q", app, got, want)
		}
	}
}
