package main

import (
	"testing"

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
