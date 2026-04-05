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
	// DegradedGracePeriod is how long SyncAndWait tolerates Synced+Degraded before
	// reporting failure. Zero is replaced by DefaultDegradedGracePeriod.
	DegradedGracePeriod time.Duration
	// AppTiers maps app name → tier string (e.g. "0-operators", "1-data", "2-services").
	// When non-nil, watchApps sequences goroutine launches by tier: all tier-0 apps
	// are watched first; tier-1 starts only after tier-0 completes successfully, etc.
	// Apps missing from the map default to DefaultTier. When nil, all apps run concurrently.
	AppTiers map[string]string
}

// DefaultTier is assigned to apps with no explicit tier.
const DefaultTier = "0-operators"

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
