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
