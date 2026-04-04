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
	Apps          []string
	Timeout       time.Duration
	Prune         bool
	SkipUnhealthy bool
	Operation     OperationType
	OnProgress    ProgressFn                      // optional, called on each state change
	ReconcileFn   func(ctx context.Context) error // triggers AppSet reconciliation
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
	Deleted    bool // true when app was confirmed deleted
}

// ExitCode indicates the overall outcome of a SyncAndWait call.
type ExitCode int

const (
	ExitSuccess ExitCode = 0
	ExitFailure ExitCode = 1
	ExitTimeout ExitCode = 2
)
