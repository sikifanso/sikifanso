package grpcsync

import (
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
)

const (
	DefaultTimeout      = 2 * time.Minute
	DefaultPollInterval = 5 * time.Second
)

// Request describes a sync-and-wait operation for one or more ArgoCD applications.
type Request struct {
	Apps          []string
	Timeout       time.Duration
	Prune         bool
	SkipUnhealthy bool
}

// Result holds the observed state of a single application after a sync operation.
type Result struct {
	App        string
	SyncStatus string
	Health     string
	Message    string
	Resources  []grpcclient.ResourceStatus
}

// ExitCode indicates the overall outcome of a SyncAndWait call.
type ExitCode int

const (
	ExitSuccess ExitCode = 0
	ExitFailure ExitCode = 1
	ExitTimeout ExitCode = 2
)
