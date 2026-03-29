package grpcclient

// AppStatus holds the summary-level status for an ArgoCD application.
type AppStatus struct {
	Name       string
	SyncStatus string
	Health     string
	Message    string
}

// ResourceStatus holds the status of a single Kubernetes resource managed by an ArgoCD application.
type ResourceStatus struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
	Status    string
	Health    string
	Message   string
}

// AppDetail combines the application-level status with its resource inventory.
type AppDetail struct {
	AppStatus
	Resources []ResourceStatus
}

// SyncOptions configures the behaviour of a sync operation.
type SyncOptions struct {
	Prune bool
}

// WatchEvent is a single event received from an application watch stream.
type WatchEvent struct {
	App     AppStatus
	Deleted bool
}

// ManagedResource describes a single Kubernetes resource that is managed by an
// ArgoCD application, together with its live/target state and diff.
type ManagedResource struct {
	Group       string
	Kind        string
	Namespace   string
	Name        string
	LiveState   string
	TargetState string
	Diff        string
}
