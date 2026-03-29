# ArgoCD gRPC Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Spec-driven workflow:** Each phase follows: spec agents write → review agents verify → implementor agents build → verifier agents check → `/simplify` between tasks.

**Goal:** Replace the fire-and-forget webhook-based ArgoCD integration with a robust gRPC client that provides real-time sync feedback, resource-level observability, rollback, and project management.

**Architecture:** Layered migration — new `internal/argocd/grpcclient/` package alongside existing REST client. Commands migrate incrementally. REST client deleted after full migration.

**Tech Stack:** `github.com/argoproj/argo-cd/v2` SDK, `google.golang.org/grpc`, Go 1.25, urfave/cli/v3

**Spec:** `docs/superpowers/specs/2026-03-29-argocd-grpc-integration-design.md`

---

## Phase 1: Infrastructure + gRPC Client

### Task 1: Add gRPC Port to HostPorts

**Files:**
- Modify: `internal/cluster/ports.go:10-24`
- Test: `internal/cluster/ports_test.go` (create if absent)

- [ ] **Step 1: Write failing test for gRPC port in HostPorts**

```go
// internal/cluster/ports_test.go
package cluster

import "testing"

func TestDefaultPortsIncludeArgoCDGRPC(t *testing.T) {
	t.Parallel()
	hp := defaultPorts
	if hp.ArgoCDGRPC == 0 {
		t.Fatal("defaultPorts.ArgoCDGRPC must be non-zero")
	}
	if hp.ArgoCDGRPC == hp.ArgoCDUI {
		t.Fatal("ArgoCDGRPC must differ from ArgoCDUI")
	}
	if hp.ArgoCDGRPC == hp.HubbleUI {
		t.Fatal("ArgoCDGRPC must differ from HubbleUI")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sikifanso && go test ./internal/cluster/ -run TestDefaultPortsIncludeArgoCDGRPC -v`
Expected: FAIL — `hp.ArgoCDGRPC undefined`

- [ ] **Step 3: Add ArgoCDGRPC field to HostPorts and defaults**

In `internal/cluster/ports.go`, update the struct (line 10) and defaults (line 18):

```go
type HostPorts struct {
	APIServer  int // default 6443 → container 6443
	HTTP       int // default 8080 → container 30082
	HTTPS      int // default 8443 → container 30083
	ArgoCDUI   int // default 30080 → container 30080
	HubbleUI   int // default 30081 → container 30081
	ArgoCDGRPC int // default 30084 → container 30084
}

var defaultPorts = HostPorts{
	APIServer:  6443,
	HTTP:       8080,
	HTTPS:      8443,
	ArgoCDUI:   30080,
	HubbleUI:   30081,
	ArgoCDGRPC: 30084,
}
```

- [ ] **Step 4: Update resolveHostPorts to allocate 6 ports instead of 5**

In `internal/cluster/ports.go`, find `resolveHostPorts()` (line 29). The function checks `allAvailable()` on defaultPorts and falls back to `findFreePorts()`. Update `findFreePorts(6)` instead of `findFreePorts(5)`, and assign the 6th port:

```go
func resolveHostPorts(log *zap.Logger) (HostPorts, error) {
	if allAvailable(defaultPorts) {
		log.Info("using default ports")
		return defaultPorts, nil
	}
	log.Info("default ports taken, allocating ephemeral ports")
	ports, err := findFreePorts(6)
	if err != nil {
		return HostPorts{}, fmt.Errorf("finding free ports: %w", err)
	}
	return HostPorts{
		APIServer:  ports[0],
		HTTP:       ports[1],
		HTTPS:      ports[2],
		ArgoCDUI:   ports[3],
		HubbleUI:   ports[4],
		ArgoCDGRPC: ports[5],
	}, nil
}
```

- [ ] **Step 5: Update allAvailable to check all 6 ports**

The `allAvailable()` function (line 57) iterates over port fields. It likely uses reflection or explicit listing. Update it to include `hp.ArgoCDGRPC`:

```go
func allAvailable(hp HostPorts) bool {
	for _, port := range []int{hp.APIServer, hp.HTTP, hp.HTTPS, hp.ArgoCDUI, hp.HubbleUI, hp.ArgoCDGRPC} {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return false
		}
		ln.Close()
	}
	return true
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd sikifanso && go test ./internal/cluster/ -run TestDefaultPortsIncludeArgoCDGRPC -v`
Expected: PASS

- [ ] **Step 7: Run all cluster tests**

Run: `cd sikifanso && go test ./internal/cluster/ -race -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/cluster/ports.go internal/cluster/ports_test.go
git commit -m "feat(ports): add ArgoCDGRPC to HostPorts for gRPC port allocation"
```

---

### Task 2: Add GRPCAddress to Session

**Files:**
- Modify: `internal/session/session.go:31-37`

- [ ] **Step 1: Write failing test**

```go
// internal/session/session_test.go (add to existing or create)
package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionRoundTrip_GRPCAddress(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sess := &Session{
		ClusterName: "grpc-test",
		State:       "running",
		Services: ServiceInfo{
			ArgoCD: ArgoCDInfo{
				URL:         "http://localhost:30080",
				GRPCAddress: "localhost:30084",
				Username:    "admin",
				Password:    "secret",
			},
		},
	}

	if err := Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("grpc-test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Services.ArgoCD.GRPCAddress != "localhost:30084" {
		t.Fatalf("GRPCAddress = %q, want %q", loaded.Services.ArgoCD.GRPCAddress, "localhost:30084")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sikifanso && go test ./internal/session/ -run TestSessionRoundTrip_GRPCAddress -v`
Expected: FAIL — `GRPCAddress` field does not exist

- [ ] **Step 3: Add GRPCAddress field to ArgoCDInfo**

In `internal/session/session.go` line 31, add the field:

```go
type ArgoCDInfo struct {
	URL          string `json:"url"`
	GRPCAddress  string `json:"grpcAddress,omitempty"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	ChartVersion string `json:"chartVersion"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sikifanso && go test ./internal/session/ -run TestSessionRoundTrip_GRPCAddress -v`
Expected: PASS

- [ ] **Step 5: Run all session tests**

Run: `cd sikifanso && go test ./internal/session/ -race -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat(session): add GRPCAddress to ArgoCDInfo"
```

---

### Task 3: Update ArgoCD Helm Values for gRPC Port

**Files:**
- Modify: `internal/infraconfig/defaults/argocd-values.yaml`
- Modify: `internal/infraconfig/overrides.go` (or wherever `ArgoCDRuntimeOverrides` is defined)

- [ ] **Step 1: Add gRPC service config to argocd-values.yaml**

In `internal/infraconfig/defaults/argocd-values.yaml`, under the `server` section, add gRPC service configuration:

```yaml
server:
  extraArgs:
    - --insecure
    - --repo-server-plaintext
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
  service:
    type: NodePort
  serviceGrpc:
    type: NodePort
```

Note: The ArgoCD Helm chart exposes `server.serviceGrpc` as a separate Service for gRPC traffic.

- [ ] **Step 2: Find and update ArgoCDRuntimeOverrides**

Locate `ArgoCDRuntimeOverrides` (in `internal/infraconfig/`). It currently sets NodePort values from resolved ports. Add the gRPC NodePort override. The function receives `HostPorts` or a similar struct — update it to set `server.serviceGrpc.nodePortGrpc` to the resolved gRPC port:

```go
// Add to the runtime overrides map:
"server.serviceGrpc.nodePortGrpc": hp.ArgoCDGRPC,
```

Note: The exact field path depends on the ArgoCD Helm chart version. Verify against the chart's `values.yaml`. The key path is typically `server.serviceGrpc.nodePortGrpc` for the `argo-cd` chart.

- [ ] **Step 3: Verify the chart accepts this value**

Run: `cd sikifanso && go test ./internal/infraconfig/ -race -v`
Expected: All PASS (or adjust if tests validate override keys)

- [ ] **Step 4: Commit**

```bash
git add internal/infraconfig/defaults/argocd-values.yaml internal/infraconfig/
git commit -m "feat(infraconfig): expose gRPC NodePort for ArgoCD server"
```

---

### Task 4: Update Cluster Creation to Store GRPCAddress

**Files:**
- Modify: `internal/cluster/cluster.go:206-212`
- Modify: `internal/cluster/k3d.go` (or wherever k3d port mappings are defined)

- [ ] **Step 1: Add gRPC port to k3d port mapping**

In the k3d cluster config builder, add a port mapping for the gRPC NodePort. Find where `hp.ArgoCDUI` is mapped (it maps host port to container NodePort). Add an analogous mapping:

```go
// Alongside existing port mappings:
{
    Port: nat.Port(fmt.Sprintf("%d/tcp", hp.ArgoCDGRPC)),
    Binding: nat.PortBinding{
        HostIP:   "0.0.0.0",
        HostPort: strconv.Itoa(hp.ArgoCDGRPC),
    },
},
```

- [ ] **Step 2: Populate GRPCAddress in session**

In `internal/cluster/cluster.go` line 206, update the session construction:

```go
Services: session.ServiceInfo{
    ArgoCD: session.ArgoCDInfo{
        URL:          fmt.Sprintf("http://localhost:%d", hp.ArgoCDUI),
        GRPCAddress:  fmt.Sprintf("localhost:%d", hp.ArgoCDGRPC),
        Username:     "admin",
        Password:     argocdResult.AdminPassword,
        ChartVersion: argocdResult.ChartVersion,
    },
    Hubble: session.HubbleInfo{
        URL: fmt.Sprintf("http://localhost:%d", hp.HubbleUI),
    },
},
```

- [ ] **Step 3: Run full build to verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add internal/cluster/
git commit -m "feat(cluster): map gRPC port in k3d and store in session"
```

---

### Task 5: Create gRPC Client — Connection and Auth

**Files:**
- Create: `internal/argocd/grpcclient/client.go`
- Create: `internal/argocd/grpcclient/client_test.go`

- [ ] **Step 1: Write failing test for client creation**

```go
// internal/argocd/grpcclient/client_test.go
package grpcclient

import (
	"context"
	"testing"
)

func TestNewClient_InvalidAddress(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := NewClient(ctx, Options{
		Address:  "localhost:0",
		Username: "admin",
		Password: "wrong",
	})
	if err == nil {
		t.Fatal("expected error for unreachable address")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sikifanso && go test ./internal/argocd/grpcclient/ -run TestNewClient_InvalidAddress -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement client.go**

```go
// internal/argocd/grpcclient/client.go
package grpcclient

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	applicationsetpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/applicationset"
	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	sessionpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
	"go.uber.org/zap"
)

type Options struct {
	Address  string
	Username string
	Password string
}

type Client struct {
	apiClient apiclient.Client
	log       *zap.Logger
}

func NewClient(ctx context.Context, opts Options) (*Client, error) {
	clientOpts := &apiclient.ClientOptions{
		ServerAddr: opts.Address,
		Insecure:   true,
		PlainText:  true,
	}

	apiClient, err := apiclient.NewClient(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("creating argocd api client: %w", err)
	}

	// Authenticate to get a JWT token
	closer, sessionClient, err := apiClient.NewSessionClient()
	if err != nil {
		return nil, fmt.Errorf("creating session client: %w", err)
	}
	defer closer.Close()

	resp, err := sessionClient.Create(ctx, &sessionpkg.SessionCreateRequest{
		Username: opts.Username,
		Password: opts.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("authenticating with argocd: %w", err)
	}

	// Recreate client with auth token
	clientOpts.AuthToken = resp.Token
	apiClient, err = apiclient.NewClient(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("creating authenticated client: %w", err)
	}

	return &Client{
		apiClient: apiClient,
	}, nil
}

func (c *Client) SetLogger(log *zap.Logger) {
	c.log = log
}

func (c *Client) Close() {
	// apiclient manages its own connections per-call via closers
}

func (c *Client) newAppClient() (applicationpkg.ApplicationServiceClient, func(), error) {
	closer, client, err := c.apiClient.NewApplicationClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating application client: %w", err)
	}
	return client, func() { closer.Close() }, nil
}

func (c *Client) newAppSetClient() (applicationsetpkg.ApplicationSetServiceClient, func(), error) {
	closer, client, err := c.apiClient.NewApplicationSetClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating applicationset client: %w", err)
	}
	return client, func() { closer.Close() }, nil
}

func (c *Client) newProjectClient() (projectpkg.ProjectServiceClient, func(), error) {
	closer, client, err := c.apiClient.NewProjectClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating project client: %w", err)
	}
	return client, func() { closer.Close() }, nil
}
```

- [ ] **Step 4: Add argo-cd/v2 dependency**

Run: `cd sikifanso && go get github.com/argoproj/argo-cd/v2@latest && go mod tidy`

Note: This may require resolving version conflicts with k8s.io dependencies. If `go mod tidy` fails, check which `k8s.io/client-go` version argo-cd/v2 requires and align.

- [ ] **Step 5: Run test**

Run: `cd sikifanso && go test ./internal/argocd/grpcclient/ -run TestNewClient_InvalidAddress -v`
Expected: PASS — connection to localhost:0 should fail with auth/connection error

- [ ] **Step 6: Run full build**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add internal/argocd/grpcclient/ go.mod go.sum
git commit -m "feat(grpcclient): add ArgoCD gRPC client with connection and auth"
```

---

### Task 6: Add ApplicationService — List and Get

**Files:**
- Create: `internal/argocd/grpcclient/applications.go`
- Create: `internal/argocd/grpcclient/types.go`
- Create: `internal/argocd/grpcclient/applications_test.go`

- [ ] **Step 1: Define domain types**

```go
// internal/argocd/grpcclient/types.go
package grpcclient

type AppStatus struct {
	Name       string
	SyncStatus string
	Health     string
	Message    string
}

type ResourceStatus struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
	Status    string
	Health    string
	Message   string
}

type AppDetail struct {
	AppStatus
	Resources []ResourceStatus
}
```

- [ ] **Step 2: Write failing test for List**

```go
// internal/argocd/grpcclient/applications_test.go
package grpcclient

import "testing"

func TestAppStatusFields(t *testing.T) {
	t.Parallel()
	s := AppStatus{
		Name:       "test",
		SyncStatus: "Synced",
		Health:     "Healthy",
		Message:    "",
	}
	if s.Name != "test" {
		t.Fatal("unexpected name")
	}
}
```

This is a basic compilation test — real integration tests require a running ArgoCD. We verify the types and method signatures compile correctly.

- [ ] **Step 3: Implement List and Get**

```go
// internal/argocd/grpcclient/applications.go
package grpcclient

import (
	"context"
	"errors"
	"fmt"

	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

var ErrAppNotFound = errors.New("application not found")

func (c *Client) ListApplications(ctx context.Context) ([]AppStatus, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	list, err := client.List(ctx, &applicationpkg.ApplicationQuery{})
	if err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}

	apps := make([]AppStatus, 0, len(list.Items))
	for _, item := range list.Items {
		apps = append(apps, toAppStatus(&item))
	}
	return apps, nil
}

func (c *Client) GetApplication(ctx context.Context, name string) (*AppDetail, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	app, err := client.Get(ctx, &applicationpkg.ApplicationQuery{
		Name: &name,
	})
	if err != nil {
		return nil, fmt.Errorf("getting application %q: %w", name, err)
	}

	detail := &AppDetail{
		AppStatus: toAppStatus(app),
	}
	return detail, nil
}

func toAppStatus(app *v1alpha1.Application) AppStatus {
	s := AppStatus{Name: app.Name}
	if app.Status.Sync.Status != "" {
		s.SyncStatus = string(app.Status.Sync.Status)
	}
	if app.Status.Health.Status != "" {
		s.Health = string(app.Status.Health.Status)
	}
	if app.Status.Health.Message != "" {
		s.Message = app.Status.Health.Message
	}
	return s
}
```

- [ ] **Step 4: Run test**

Run: `cd sikifanso && go test ./internal/argocd/grpcclient/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/argocd/grpcclient/
git commit -m "feat(grpcclient): add List and Get for ApplicationService"
```

---

### Task 7: Add ApplicationService — Sync and Watch

**Files:**
- Modify: `internal/argocd/grpcclient/applications.go`

- [ ] **Step 1: Add SyncOptions type to types.go**

```go
// Append to internal/argocd/grpcclient/types.go

type SyncOptions struct {
	Prune bool
}

type WatchEvent struct {
	App     AppStatus
	Deleted bool
}
```

- [ ] **Step 2: Implement Sync**

Append to `internal/argocd/grpcclient/applications.go`:

```go
func (c *Client) SyncApplication(ctx context.Context, name string, opts SyncOptions) error {
	client, closer, err := c.newAppClient()
	if err != nil {
		return err
	}
	defer closer()

	_, err = client.Sync(ctx, &applicationpkg.ApplicationSyncRequest{
		Name:  &name,
		Prune: &opts.Prune,
	})
	if err != nil {
		return fmt.Errorf("syncing application %q: %w", name, err)
	}
	return nil
}
```

- [ ] **Step 3: Implement Watch**

Append to `internal/argocd/grpcclient/applications.go`:

```go
func (c *Client) WatchApplication(ctx context.Context, name string) (<-chan WatchEvent, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}

	stream, err := client.Watch(ctx, &applicationpkg.ApplicationQuery{
		Name: &name,
	})
	if err != nil {
		closer()
		return nil, fmt.Errorf("watching application %q: %w", name, err)
	}

	ch := make(chan WatchEvent, 16)
	go func() {
		defer closer()
		defer close(ch)
		for {
			event, err := stream.Recv()
			if err != nil {
				return
			}
			we := WatchEvent{}
			if event.Application != (v1alpha1.Application{}) {
				we.App = toAppStatus(&event.Application)
			}
			if event.Type == "DELETED" {
				we.Deleted = true
			}
			select {
			case ch <- we:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd sikifanso && go build ./internal/argocd/grpcclient/`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add internal/argocd/grpcclient/
git commit -m "feat(grpcclient): add Sync and Watch for ApplicationService"
```

---

### Task 8: Add ApplicationService — ResourceTree, ManagedResources

**Files:**
- Modify: `internal/argocd/grpcclient/applications.go`
- Modify: `internal/argocd/grpcclient/types.go`

- [ ] **Step 1: Add ManagedResource type**

Append to `internal/argocd/grpcclient/types.go`:

```go
type ManagedResource struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
	LiveState string // JSON of live resource
	TargetState string // JSON of desired resource
	Diff      string
}
```

- [ ] **Step 2: Implement ResourceTree**

Append to `internal/argocd/grpcclient/applications.go`:

```go
func (c *Client) ResourceTree(ctx context.Context, name string) ([]ResourceStatus, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	tree, err := client.ResourceTree(ctx, &applicationpkg.ResourcesQuery{
		ApplicationName: &name,
	})
	if err != nil {
		return nil, fmt.Errorf("getting resource tree for %q: %w", name, err)
	}

	resources := make([]ResourceStatus, 0, len(tree.Nodes))
	for _, node := range tree.Nodes {
		rs := ResourceStatus{
			Group:     node.Group,
			Kind:      node.Kind,
			Namespace: node.Namespace,
			Name:      node.Name,
		}
		if node.Health != nil {
			rs.Health = string(node.Health.Status)
			rs.Message = node.Health.Message
		}
		resources = append(resources, rs)
	}
	return resources, nil
}
```

- [ ] **Step 3: Implement ManagedResources**

Append to `internal/argocd/grpcclient/applications.go`:

```go
func (c *Client) ManagedResources(ctx context.Context, name string) ([]ManagedResource, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	resp, err := client.ManagedResources(ctx, &applicationpkg.ResourcesQuery{
		ApplicationName: &name,
	})
	if err != nil {
		return nil, fmt.Errorf("getting managed resources for %q: %w", name, err)
	}

	resources := make([]ManagedResource, 0, len(resp.Items))
	for _, item := range resp.Items {
		mr := ManagedResource{
			Group:       item.Group,
			Kind:        item.Kind,
			Namespace:   item.Namespace,
			Name:        item.Name,
			LiveState:   item.LiveState,
			TargetState: item.TargetState,
			Diff:        item.Diff.NormalizedLiveState,
		}
		resources = append(resources, mr)
	}
	return resources, nil
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd sikifanso && go build ./internal/argocd/grpcclient/`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add internal/argocd/grpcclient/
git commit -m "feat(grpcclient): add ResourceTree and ManagedResources"
```

---

### Task 9: Add ApplicationService — Rollback, Delete, PodLogs, RunAction

**Files:**
- Modify: `internal/argocd/grpcclient/applications.go`

- [ ] **Step 1: Implement Rollback**

```go
func (c *Client) Rollback(ctx context.Context, name string, revisionID int64) error {
	client, closer, err := c.newAppClient()
	if err != nil {
		return err
	}
	defer closer()

	_, err = client.Rollback(ctx, &applicationpkg.ApplicationRollbackRequest{
		Name: &name,
		Id:   &revisionID,
	})
	if err != nil {
		return fmt.Errorf("rolling back %q to revision %d: %w", name, revisionID, err)
	}
	return nil
}
```

- [ ] **Step 2: Implement Delete**

```go
func (c *Client) DeleteApplication(ctx context.Context, name string, cascade bool) error {
	client, closer, err := c.newAppClient()
	if err != nil {
		return err
	}
	defer closer()

	_, err = client.Delete(ctx, &applicationpkg.ApplicationDeleteRequest{
		Name:    &name,
		Cascade: &cascade,
	})
	if err != nil {
		return fmt.Errorf("deleting application %q: %w", name, err)
	}
	return nil
}
```

- [ ] **Step 3: Implement PodLogs**

```go
func (c *Client) PodLogs(ctx context.Context, name, podName, container string, follow bool) (<-chan string, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}

	stream, err := client.PodLogs(ctx, &applicationpkg.ApplicationPodLogsQuery{
		Name:         &name,
		PodName:      &podName,
		Container:    &container,
		Follow:       &follow,
	})
	if err != nil {
		closer()
		return nil, fmt.Errorf("streaming logs for %s/%s: %w", name, podName, err)
	}

	ch := make(chan string, 64)
	go func() {
		defer closer()
		defer close(ch)
		for {
			entry, err := stream.Recv()
			if err != nil {
				return
			}
			select {
			case ch <- entry.Content:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
```

- [ ] **Step 4: Implement RunAction**

```go
func (c *Client) RunResourceAction(ctx context.Context, appName, namespace, resourceName, group, kind, action string) error {
	client, closer, err := c.newAppClient()
	if err != nil {
		return err
	}
	defer closer()

	_, err = client.RunResourceAction(ctx, &applicationpkg.ResourceActionRunRequest{
		Name:         &appName,
		Namespace:    &namespace,
		ResourceName: &resourceName,
		Group:        &group,
		Kind:         &kind,
		Action:       &action,
	})
	if err != nil {
		return fmt.Errorf("running action %q on %s/%s: %w", action, kind, resourceName, err)
	}
	return nil
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd sikifanso && go build ./internal/argocd/grpcclient/`
Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add internal/argocd/grpcclient/
git commit -m "feat(grpcclient): add Rollback, Delete, PodLogs, RunAction"
```

---

## Phase 2: Sync Rewrite

### Task 10: Define New Sync Types

**Files:**
- Create: `internal/argocd/grpcsync/types.go`

- [ ] **Step 1: Create sync types**

```go
// internal/argocd/grpcsync/types.go
package grpcsync

import (
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
)

const (
	DefaultTimeout      = 2 * time.Minute
	DefaultPollInterval = 5 * time.Second
)

type Request struct {
	Apps          []string
	Timeout       time.Duration
	Prune         bool
	SkipUnhealthy bool
}

type Result struct {
	App        string
	SyncStatus string
	Health     string
	Message    string
	Resources  []grpcclient.ResourceStatus
}

type ExitCode int

const (
	ExitSuccess  ExitCode = 0
	ExitFailure  ExitCode = 1
	ExitTimeout  ExitCode = 2
)
```

- [ ] **Step 2: Verify compilation**

Run: `cd sikifanso && go build ./internal/argocd/grpcsync/`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add internal/argocd/grpcsync/
git commit -m "feat(grpcsync): define sync request/result types"
```

---

### Task 11: Implement Sync Orchestrator

**Files:**
- Create: `internal/argocd/grpcsync/orchestrator.go`
- Create: `internal/argocd/grpcsync/orchestrator_test.go`

- [ ] **Step 1: Write failing test for sync orchestrator**

```go
// internal/argocd/grpcsync/orchestrator_test.go
package grpcsync

import (
	"testing"
	"time"
)

func TestRequestDefaults(t *testing.T) {
	t.Parallel()
	r := Request{
		Apps:    []string{"foo"},
		Timeout: 0,
	}
	if r.Timeout == 0 {
		r.Timeout = DefaultTimeout
	}
	if r.Timeout != 2*time.Minute {
		t.Fatalf("default timeout = %v, want 2m", r.Timeout)
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
```

- [ ] **Step 2: Run test**

Run: `cd sikifanso && go test ./internal/argocd/grpcsync/ -v`
Expected: PASS

- [ ] **Step 3: Implement the orchestrator**

```go
// internal/argocd/grpcsync/orchestrator.go
package grpcsync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"go.uber.org/zap"
)

type Orchestrator struct {
	client *grpcclient.Client
	log    *zap.Logger
}

func NewOrchestrator(client *grpcclient.Client, log *zap.Logger) *Orchestrator {
	return &Orchestrator{client: client, log: log}
}

// SyncAndWait triggers sync for each app, then watches until all are Synced+Healthy or timeout.
func (o *Orchestrator) SyncAndWait(ctx context.Context, req Request) ([]Result, ExitCode) {
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}
	if !req.Prune {
		req.Prune = true
	}

	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Trigger sync for each app
	for _, app := range req.Apps {
		if err := o.client.SyncApplication(ctx, app, grpcclient.SyncOptions{Prune: req.Prune}); err != nil {
			o.log.Warn("sync trigger failed", zap.String("app", app), zap.Error(err))
		}
	}

	// Watch all apps concurrently
	return o.watchApps(ctx, req)
}

// SyncOnly triggers sync without waiting.
func (o *Orchestrator) SyncOnly(ctx context.Context, req Request) error {
	for _, app := range req.Apps {
		if err := o.client.SyncApplication(ctx, app, grpcclient.SyncOptions{Prune: req.Prune}); err != nil {
			return fmt.Errorf("syncing %q: %w", app, err)
		}
	}
	return nil
}

func (o *Orchestrator) watchApps(ctx context.Context, req Request) ([]Result, ExitCode) {
	var mu sync.Mutex
	results := make(map[string]*Result, len(req.Apps))
	for _, app := range req.Apps {
		results[app] = &Result{App: app}
	}

	var wg sync.WaitGroup
	for _, app := range req.Apps {
		wg.Add(1)
		go func(appName string) {
			defer wg.Done()
			o.watchSingleApp(ctx, appName, req.SkipUnhealthy, func(r Result) {
				mu.Lock()
				defer mu.Unlock()
				results[appName] = &r
			})
		}(app)
	}

	// Wait for all watchers to complete (they exit on Synced+Healthy, Degraded, or context cancellation)
	wg.Wait()

	// Collect results
	exitCode := ExitSuccess
	finalResults := make([]Result, 0, len(results))
	for _, r := range results {
		finalResults = append(finalResults, *r)
		switch {
		case r.Health == "Degraded" || r.Health == "Missing":
			if !req.SkipUnhealthy {
				exitCode = ExitFailure
			}
		case r.SyncStatus == "OutOfSync":
			exitCode = ExitFailure
		case r.Health == "Progressing" || r.Health == "Unknown" || r.Health == "":
			if exitCode != ExitFailure {
				exitCode = ExitTimeout
			}
		}
	}

	return finalResults, exitCode
}

func (o *Orchestrator) watchSingleApp(ctx context.Context, name string, skipUnhealthy bool, update func(Result)) {
	ch, err := o.client.WatchApplication(ctx, name)
	if err != nil {
		o.log.Error("failed to watch app", zap.String("app", name), zap.Error(err))
		// Fallback: poll once for current state
		o.pollOnce(ctx, name, update)
		return
	}

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			if event.Deleted {
				update(Result{App: name, Message: "application deleted"})
				return
			}
			r := Result{
				App:        name,
				SyncStatus: event.App.SyncStatus,
				Health:     event.App.Health,
				Message:    event.App.Message,
			}
			update(r)
			o.log.Info("app status",
				zap.String("app", name),
				zap.String("sync", r.SyncStatus),
				zap.String("health", r.Health),
			)

			if r.SyncStatus == "Synced" && r.Health == "Healthy" {
				return
			}
			if r.Health == "Degraded" && !skipUnhealthy {
				// Fetch resource tree for failure details
				resources, rtErr := o.client.ResourceTree(ctx, name)
				if rtErr == nil {
					r.Resources = resources
					update(r)
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (o *Orchestrator) pollOnce(ctx context.Context, name string, update func(Result)) {
	detail, err := o.client.GetApplication(ctx, name)
	if err != nil {
		o.log.Error("poll fallback failed", zap.String("app", name), zap.Error(err))
		return
	}
	update(Result{
		App:        name,
		SyncStatus: detail.SyncStatus,
		Health:     detail.Health,
		Message:    detail.Message,
	})
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd sikifanso && go build ./internal/argocd/grpcsync/`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add internal/argocd/grpcsync/
git commit -m "feat(grpcsync): implement sync orchestrator with watch-based waiting"
```

---

### Task 12: Update syncAfterMutation Middleware

**Files:**
- Modify: `cmd/sikifanso/middleware.go:44-80`

- [ ] **Step 1: Add gRPC client creation helper to middleware**

Add a helper that creates a gRPC client from session. Insert above `syncAfterMutation`:

```go
func grpcClientFromSession(ctx context.Context, sess *session.Session) (*grpcclient.Client, error) {
	if sess.Services.ArgoCD.GRPCAddress == "" {
		return nil, fmt.Errorf("no gRPC address in session — cluster may need recreation")
	}
	return grpcclient.NewClient(ctx, grpcclient.Options{
		Address:  sess.Services.ArgoCD.GRPCAddress,
		Username: sess.Services.ArgoCD.Username,
		Password: sess.Services.ArgoCD.Password,
	})
}
```

- [ ] **Step 2: Rewrite syncAfterMutation to use gRPC**

Replace `syncAfterMutation` (lines 64-80) with:

```go
func syncAfterMutation(ctx context.Context, cmd *cli.Command, sess *session.Session, apps ...string) {
	// Fallback to old REST sync if no gRPC address (backward compat during migration)
	if sess.Services.ArgoCD.GRPCAddress == "" {
		opts := syncOptsFromSession(sess)
		argocd.SyncAndReport(ctx, zapLogger, os.Stderr, opts)
		return
	}

	client, err := grpcClientFromSession(ctx, sess)
	if err != nil {
		zapLogger.Warn("gRPC client failed, falling back to REST sync", zap.Error(err))
		opts := syncOptsFromSession(sess)
		argocd.SyncAndReport(ctx, zapLogger, os.Stderr, opts)
		return
	}
	defer client.Close()

	orch := grpcsync.NewOrchestrator(client, zapLogger)

	noWait := !cmd.Bool("wait")
	req := grpcsync.Request{
		Apps:    apps,
		Timeout: cmd.Duration("timeout"),
		Prune:   true,
	}

	if noWait {
		if err := orch.SyncOnly(ctx, req); err != nil {
			zapLogger.Warn("sync trigger failed", zap.Error(err))
		}
		fmt.Fprintln(os.Stderr, "ArgoCD sync triggered")
		return
	}

	results, exitCode := orch.SyncAndWait(ctx, req)
	printSyncResults(os.Stderr, results)
	if exitCode != grpcsync.ExitSuccess {
		zapLogger.Warn("sync completed with issues", zap.Int("exitCode", int(exitCode)))
	}
}
```

- [ ] **Step 3: Add printSyncResults helper**

```go
func printSyncResults(w io.Writer, results []grpcsync.Result) {
	for _, r := range results {
		indicator := "✓"
		if r.Health == "Degraded" || r.Health == "Missing" || r.SyncStatus == "OutOfSync" {
			indicator = "✗"
		} else if r.Health == "Progressing" || r.Health == "Unknown" {
			indicator = "~"
		}
		fmt.Fprintf(w, "  %s %s: %s/%s\n", indicator, r.App, r.SyncStatus, r.Health)
		for _, res := range r.Resources {
			if res.Health != "" && res.Health != "Healthy" {
				fmt.Fprintf(w, "    └─ %s/%s: %s", res.Kind, res.Name, res.Health)
				if res.Message != "" {
					fmt.Fprintf(w, " (%s)", res.Message)
				}
				fmt.Fprintln(w)
			}
		}
	}
}
```

- [ ] **Step 4: Update imports**

Add imports for `grpcclient`, `grpcsync`, and `io` packages.

- [ ] **Step 5: Update waitSyncFlags to change --wait default**

The design says default is now "wait" (block). Update flags so `--no-wait` is the opt-out:

```go
func waitSyncFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "wait", Usage: "Wait for apps to reach Synced/Healthy after sync (default true)", Value: true},
		&cli.BoolFlag{Name: "no-wait", Usage: "Trigger sync without waiting"},
		&cli.DurationFlag{Name: "timeout", Usage: "Timeout for sync wait", Value: grpcsync.DefaultTimeout},
	}
}
```

Update `syncAfterMutation` to check `cmd.Bool("no-wait")` instead of `!cmd.Bool("wait")`.

- [ ] **Step 6: Verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add cmd/sikifanso/middleware.go
git commit -m "feat(middleware): rewrite syncAfterMutation to use gRPC with REST fallback"
```

---

### Task 13: Update ArgoCD Sync Command

**Files:**
- Modify: `cmd/sikifanso/argocd_sync.go`

- [ ] **Step 1: Rewrite argocdSyncCmd to use gRPC orchestrator**

Replace the action in `argocdSyncCmd()`:

```go
func argocdSyncCmd() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Trigger ArgoCD sync for all or specific applications",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "no-wait", Usage: "Trigger sync without waiting for completion"},
			&cli.StringFlag{Name: "app", Usage: "Sync a specific application by name"},
			&cli.DurationFlag{Name: "timeout", Usage: "Timeout for sync wait", Value: grpcsync.DefaultTimeout},
			&cli.BoolFlag{Name: "skip-unhealthy", Usage: "Ignore pre-existing Degraded apps"},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return fmt.Errorf("connecting to ArgoCD gRPC: %w", err)
			}
			defer client.Close()

			orch := grpcsync.NewOrchestrator(client, zapLogger)

			req := grpcsync.Request{
				Timeout:       cmd.Duration("timeout"),
				Prune:         true,
				SkipUnhealthy: cmd.Bool("skip-unhealthy"),
			}

			if appName := cmd.String("app"); appName != "" {
				req.Apps = []string{appName}
			}

			if cmd.Bool("no-wait") {
				if err := orch.SyncOnly(ctx, req); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "ArgoCD sync triggered")
				return nil
			}

			results, exitCode := orch.SyncAndWait(ctx, req)
			printSyncResults(os.Stderr, results)

			if exitCode == grpcsync.ExitFailure {
				return fmt.Errorf("sync failed: one or more apps unhealthy")
			}
			if exitCode == grpcsync.ExitTimeout {
				return fmt.Errorf("sync timed out: apps still progressing")
			}
			return nil
		}),
	}
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/sikifanso/argocd_sync.go
git commit -m "feat(argocd): rewrite sync command to use gRPC orchestrator"
```

---

### Task 14: Delete Webhook Logic

**Files:**
- Modify: `internal/argocd/sync.go` — remove webhook functions
- Modify: `internal/mcp/helpers.go` — update triggerSync

- [ ] **Step 1: Remove sendWebhook and sendAppSetWebhook from sync.go**

Delete `sendWebhook()` (lines 288-307) and `sendAppSetWebhook()` (lines 311-338) from `internal/argocd/sync.go`. Also remove the `webhookPayload` constant (line 17).

- [ ] **Step 2: Update MCP triggerSync helper**

In `internal/mcp/helpers.go` line 46, `triggerSync` currently calls the webhook-based sync. Update it to use gRPC:

```go
func triggerSync(ctx context.Context, deps *Deps, sess *session.Session) error {
	if sess.Services.ArgoCD.GRPCAddress == "" {
		return argocd.Sync(ctx, deps.Logger, sess.ClusterName, sess.Services.ArgoCD.URL)
	}
	client, err := grpcclient.NewClient(ctx, grpcclient.Options{
		Address:  sess.Services.ArgoCD.GRPCAddress,
		Username: sess.Services.ArgoCD.Username,
		Password: sess.Services.ArgoCD.Password,
	})
	if err != nil {
		return fmt.Errorf("gRPC client: %w", err)
	}
	defer client.Close()

	// List all apps and sync them
	apps, err := client.ListApplications(ctx)
	if err != nil {
		return fmt.Errorf("listing apps for sync: %w", err)
	}
	for _, app := range apps {
		_ = client.SyncApplication(ctx, app.Name, grpcclient.SyncOptions{Prune: true})
	}
	return nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 4: Run all tests**

Run: `cd sikifanso && go test ./... -race`
Expected: All PASS (some old sync tests may need updating — fix any failures)

- [ ] **Step 5: Commit**

```bash
git add internal/argocd/sync.go internal/mcp/helpers.go
git commit -m "refactor(argocd): remove webhook logic, use gRPC sync in MCP"
```

---

## Phase 3: Observability

### Task 15: Enhance Doctor with Resource Tree

**Files:**
- Modify: `internal/doctor/argocd.go`
- Modify: `internal/doctor/apps.go`
- Modify: `internal/doctor/doctor.go`

- [ ] **Step 1: Add gRPC client option to AppsCheck**

In `internal/doctor/apps.go`, the `AppsCheck` struct uses a dynamic client to read Application CRDs. Add an optional gRPC client field so it can fetch resource tree details:

```go
type AppsCheck struct {
	DynClient       dynamic.Interface
	GitOpsPath      string
	ArgoCDNamespace string
	GRPCClient      *grpcclient.Client // optional, enriches results with resource tree
}
```

- [ ] **Step 2: Enrich per-app results with resource tree**

In the `Run()` method of `AppsCheck`, after detecting an unhealthy app, if `GRPCClient` is non-nil, fetch the resource tree and append failing resources to the result message:

```go
if c.GRPCClient != nil && !healthy {
	resources, err := c.GRPCClient.ResourceTree(ctx, appName)
	if err == nil {
		for _, res := range resources {
			if res.Health != "" && res.Health != "Healthy" {
				detail := fmt.Sprintf("  └─ %s/%s: %s", res.Kind, res.Name, res.Health)
				if res.Message != "" {
					detail += fmt.Sprintf(" (%s)", res.Message)
				}
				result.Cause += "\n" + detail
			}
		}
	}
}
```

- [ ] **Step 3: Wire gRPC client in AppChecks factory**

In `internal/doctor/doctor.go`, update `AppChecks()` to accept optional gRPC client:

```go
func AppChecks(dynClient dynamic.Interface, gitOpsPath string, cfg *infraconfig.InfraConfig, grpcClient *grpcclient.Client) []Check {
	return []Check{
		AppsCheck{DynClient: dynClient, GitOpsPath: gitOpsPath, ArgoCDNamespace: cfg.ArgoCD.Namespace, GRPCClient: grpcClient},
		AgentsCheck{DynClient: dynClient, GitOpsPath: gitOpsPath, ArgoCDNamespace: cfg.ArgoCD.Namespace},
	}
}
```

- [ ] **Step 4: Update all callers of AppChecks**

Update `cmd/sikifanso/doctor.go` and `internal/mcp/doctor.go` to pass gRPC client (or nil if unavailable):

```go
// In cmd/sikifanso/doctor.go, after creating dynClient:
var grpcClient *grpcclient.Client
if sess.Services.ArgoCD.GRPCAddress != "" {
	grpcClient, _ = grpcclient.NewClient(ctx, grpcclient.Options{
		Address:  sess.Services.ArgoCD.GRPCAddress,
		Username: sess.Services.ArgoCD.Username,
		Password: sess.Services.ArgoCD.Password,
	})
	if grpcClient != nil {
		defer grpcClient.Close()
	}
}
checks = append(checks, doctor.AppChecks(dynClient, sess.GitOpsPath, cfg, grpcClient)...)
```

- [ ] **Step 5: Verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 6: Run doctor tests**

Run: `cd sikifanso && go test ./internal/doctor/ -race -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/doctor/ cmd/sikifanso/doctor.go internal/mcp/doctor.go
git commit -m "feat(doctor): enrich app health checks with resource tree details"
```

---

### Task 16: Add `argocd status` CLI Command

**Files:**
- Create: `cmd/sikifanso/argocd_status.go`
- Modify: `cmd/sikifanso/argocd_cmd.go`

- [ ] **Step 1: Create argocd status command**

```go
// cmd/sikifanso/argocd_status.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdStatusCmd() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show detailed application status with resource tree",
		ArgsUsage: "[APP]",
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			appName := cmd.Args().First()
			if appName != "" {
				return printAppDetail(ctx, client, appName)
			}
			return printAllAppsStatus(ctx, client)
		}),
	}
}

func printAllAppsStatus(ctx context.Context, client *grpcclient.Client) error {
	apps, err := client.ListApplications(ctx)
	if err != nil {
		return err
	}
	for _, app := range apps {
		indicator := statusIndicator(app.Health, app.SyncStatus)
		fmt.Fprintf(os.Stderr, "  %s %-30s %s/%s\n", indicator, app.Name, app.SyncStatus, app.Health)
	}
	return nil
}

func printAppDetail(ctx context.Context, client *grpcclient.Client, name string) error {
	detail, err := client.GetApplication(ctx, name)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Application: %s\n", detail.Name)
	fmt.Fprintf(os.Stderr, "Sync:        %s\n", detail.SyncStatus)
	fmt.Fprintf(os.Stderr, "Health:      %s\n", detail.Health)
	if detail.Message != "" {
		fmt.Fprintf(os.Stderr, "Message:     %s\n", detail.Message)
	}

	resources, err := client.ResourceTree(ctx, name)
	if err != nil {
		return err
	}

	if len(resources) > 0 {
		fmt.Fprintln(os.Stderr, "\nResources:")
		for _, r := range resources {
			health := r.Health
			if health == "" {
				health = "-"
			}
			fmt.Fprintf(os.Stderr, "  %-20s %-30s %s", r.Kind, r.Name, health)
			if r.Message != "" {
				fmt.Fprintf(os.Stderr, " (%s)", r.Message)
			}
			fmt.Fprintln(os.Stderr)
		}
	}
	return nil
}

func statusIndicator(health, sync string) string {
	switch {
	case sync == "Synced" && health == "Healthy":
		return "✓"
	case health == "Degraded" || health == "Missing":
		return "✗"
	default:
		return "~"
	}
}
```

- [ ] **Step 2: Register in argocdCmd**

In `cmd/sikifanso/argocd_cmd.go`, add `argocdStatusCmd()` to the Commands slice:

```go
func argocdCmd() *cli.Command {
	return &cli.Command{
		Name:  "argocd",
		Usage: "ArgoCD operations",
		Commands: []*cli.Command{
			argocdSyncCmd(),
			argocdStatusCmd(),
		},
	}
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add cmd/sikifanso/argocd_status.go cmd/sikifanso/argocd_cmd.go
git commit -m "feat(cli): add argocd status command with resource tree"
```

---

### Task 17: Add `argocd diff` CLI Command

**Files:**
- Create: `cmd/sikifanso/argocd_diff.go`
- Modify: `cmd/sikifanso/argocd_cmd.go`

- [ ] **Step 1: Create argocd diff command**

```go
// cmd/sikifanso/argocd_diff.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdDiffCmd() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show live vs desired state diff for an application",
		ArgsUsage: "APP",
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("app name required: sikifanso argocd diff APP")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			resources, err := client.ManagedResources(ctx, name)
			if err != nil {
				return err
			}

			hasDiff := false
			for _, r := range resources {
				if r.LiveState != r.TargetState && r.TargetState != "" && r.LiveState != "" {
					hasDiff = true
					fmt.Fprintf(os.Stdout, "--- %s/%s (live)\n+++ %s/%s (desired)\n", r.Kind, r.Name, r.Kind, r.Name)
					fmt.Fprintln(os.Stdout, r.Diff)
					fmt.Fprintln(os.Stdout)
				}
			}

			if !hasDiff {
				fmt.Fprintln(os.Stderr, "No differences found — live state matches desired state.")
			}
			return nil
		}),
	}
}
```

- [ ] **Step 2: Register in argocdCmd**

Add `argocdDiffCmd()` to the Commands slice in `argocd_cmd.go`.

- [ ] **Step 3: Verify and commit**

Run: `cd sikifanso && go build ./cmd/sikifanso`

```bash
git add cmd/sikifanso/argocd_diff.go cmd/sikifanso/argocd_cmd.go
git commit -m "feat(cli): add argocd diff command for live vs desired state"
```

---

### Task 18: Add `argocd logs` CLI Command

**Files:**
- Create: `cmd/sikifanso/argocd_logs.go`
- Modify: `cmd/sikifanso/argocd_cmd.go`

- [ ] **Step 1: Create argocd logs command**

```go
// cmd/sikifanso/argocd_logs.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdLogsCmd() *cli.Command {
	return &cli.Command{
		Name:      "logs",
		Usage:     "Stream pod logs for an ArgoCD application",
		ArgsUsage: "APP",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "pod", Usage: "Pod name (required)"},
			&cli.StringFlag{Name: "container", Usage: "Container name", Value: ""},
			&cli.BoolFlag{Name: "follow", Aliases: []string{"f"}, Usage: "Follow log output"},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("app name required: sikifanso argocd logs APP --pod POD")
			}
			podName := cmd.String("pod")
			if podName == "" {
				return fmt.Errorf("--pod flag is required")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			ch, err := client.PodLogs(ctx, name, podName, cmd.String("container"), cmd.Bool("follow"))
			if err != nil {
				return err
			}

			for line := range ch {
				fmt.Fprintln(os.Stdout, line)
			}
			return nil
		}),
	}
}
```

- [ ] **Step 2: Register in argocdCmd**

Add `argocdLogsCmd()` to the Commands slice.

- [ ] **Step 3: Verify and commit**

Run: `cd sikifanso && go build ./cmd/sikifanso`

```bash
git add cmd/sikifanso/argocd_logs.go cmd/sikifanso/argocd_cmd.go
git commit -m "feat(cli): add argocd logs command for pod log streaming"
```

---

### Task 19: Add `argocd rollback` CLI Command

**Files:**
- Create: `cmd/sikifanso/argocd_rollback.go`
- Modify: `cmd/sikifanso/argocd_cmd.go`

- [ ] **Step 1: Create argocd rollback command**

```go
// cmd/sikifanso/argocd_rollback.go
package main

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdRollbackCmd() *cli.Command {
	return &cli.Command{
		Name:      "rollback",
		Usage:     "Rollback an application to a previous sync revision",
		ArgsUsage: "APP",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "revision", Usage: "Revision ID to rollback to (default: previous)", Value: 0},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("app name required: sikifanso argocd rollback APP")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			revisionID := int64(cmd.Int("revision"))
			if err := client.Rollback(ctx, name, revisionID); err != nil {
				return err
			}

			fmt.Fprintf(cmd.Root().Writer, "Rolled back %s to revision %d\n", name, revisionID)
			return nil
		}),
	}
}
```

- [ ] **Step 2: Register in argocdCmd**

Add `argocdRollbackCmd()` to the Commands slice.

- [ ] **Step 3: Verify and commit**

Run: `cd sikifanso && go build ./cmd/sikifanso`

```bash
git add cmd/sikifanso/argocd_rollback.go cmd/sikifanso/argocd_cmd.go
git commit -m "feat(cli): add argocd rollback command"
```

---

### Task 20: Add and Update MCP Tools

**Files:**
- Modify: `internal/mcp/doctor.go`
- Create: `internal/mcp/argocd.go`

- [ ] **Step 1: Create argocd MCP tools file**

```go
// internal/mcp/argocd.go
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type argocdAppInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Name    string `json:"name" jsonschema:"Application name"`
}

type argocdRollbackInput struct {
	Cluster  string `json:"cluster" jsonschema:"Name of the cluster"`
	Name     string `json:"name" jsonschema:"Application name"`
	Revision int64  `json:"revision" jsonschema:"Revision ID to rollback to"`
}

func registerArgoCDTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_app_detail",
		Description: "Get detailed application status including resource tree",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdAppInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if e != nil {
			return r, sv, e
		}

		client, err := grpcclient.NewClient(ctx, grpcclient.Options{
			Address:  sess.Services.ArgoCD.GRPCAddress,
			Username: sess.Services.ArgoCD.Username,
			Password: sess.Services.ArgoCD.Password,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("gRPC client: %w", err)
		}
		defer client.Close()

		detail, err := client.GetApplication(ctx, input.Name)
		if err != nil {
			return nil, nil, err
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Application: %s\nSync: %s\nHealth: %s\n", detail.Name, detail.SyncStatus, detail.Health)
		if detail.Message != "" {
			fmt.Fprintf(&sb, "Message: %s\n", detail.Message)
		}

		resources, err := client.ResourceTree(ctx, input.Name)
		if err == nil && len(resources) > 0 {
			sb.WriteString("\nResources:\n")
			for _, r := range resources {
				health := r.Health
				if health == "" {
					health = "-"
				}
				fmt.Fprintf(&sb, "  %-20s %-30s %s", r.Kind, r.Name, health)
				if r.Message != "" {
					fmt.Fprintf(&sb, " (%s)", r.Message)
				}
				sb.WriteString("\n")
			}
		}

		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_app_diff",
		Description: "Show live vs desired state diff for an application",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdAppInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if e != nil {
			return r, sv, e
		}

		client, err := grpcclient.NewClient(ctx, grpcclient.Options{
			Address:  sess.Services.ArgoCD.GRPCAddress,
			Username: sess.Services.ArgoCD.Username,
			Password: sess.Services.ArgoCD.Password,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("gRPC client: %w", err)
		}
		defer client.Close()

		resources, err := client.ManagedResources(ctx, input.Name)
		if err != nil {
			return nil, nil, err
		}

		var sb strings.Builder
		for _, res := range resources {
			if res.LiveState != res.TargetState && res.TargetState != "" && res.LiveState != "" {
				fmt.Fprintf(&sb, "--- %s/%s (live)\n+++ %s/%s (desired)\n%s\n\n", res.Kind, res.Name, res.Kind, res.Name, res.Diff)
			}
		}

		if sb.Len() == 0 {
			return textResult("No differences — live state matches desired state.")
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_rollback",
		Description: "Rollback an application to a previous sync revision",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdRollbackInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if e != nil {
			return r, sv, e
		}

		client, err := grpcclient.NewClient(ctx, grpcclient.Options{
			Address:  sess.Services.ArgoCD.GRPCAddress,
			Username: sess.Services.ArgoCD.Username,
			Password: sess.Services.ArgoCD.Password,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("gRPC client: %w", err)
		}
		defer client.Close()

		if err := client.Rollback(ctx, input.Name, input.Revision); err != nil {
			return nil, nil, err
		}

		return textResult(fmt.Sprintf("Rolled back %s to revision %d", input.Name, input.Revision))
	})
}
```

- [ ] **Step 2: Register in server.go**

In `internal/mcp/server.go`, add `registerArgoCDTools(s, deps)` alongside existing registrations.

- [ ] **Step 3: Update argocd_apps tool to use gRPC when available**

In `internal/mcp/doctor.go`, update the `argocd_apps` tool to prefer gRPC client over REST, falling back to REST if GRPCAddress is empty.

- [ ] **Step 4: Verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/
git commit -m "feat(mcp): add argocd_app_detail, argocd_app_diff, argocd_rollback tools"
```

---

## Phase 4: ApplicationSet + Project Services

### Task 21: Add ApplicationSetService to gRPC Client

**Files:**
- Create: `internal/argocd/grpcclient/applicationsets.go`

- [ ] **Step 1: Define types**

Append to `internal/argocd/grpcclient/types.go`:

```go
type AppSetSummary struct {
	Name       string
	Namespace  string
	Apps       int // number of generated applications
}
```

- [ ] **Step 2: Implement ApplicationSet methods**

```go
// internal/argocd/grpcclient/applicationsets.go
package grpcclient

import (
	"context"
	"fmt"

	applicationsetpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/applicationset"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

func (c *Client) ListApplicationSets(ctx context.Context) ([]AppSetSummary, error) {
	client, closer, err := c.newAppSetClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	list, err := client.List(ctx, &applicationsetpkg.ApplicationSetListQuery{})
	if err != nil {
		return nil, fmt.Errorf("listing applicationsets: %w", err)
	}

	result := make([]AppSetSummary, 0, len(list.Items))
	for _, item := range list.Items {
		result = append(result, AppSetSummary{
			Name:      item.Name,
			Namespace: item.Namespace,
		})
	}
	return result, nil
}

func (c *Client) GetApplicationSet(ctx context.Context, name string) (*v1alpha1.ApplicationSet, error) {
	client, closer, err := c.newAppSetClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	appSet, err := client.Get(ctx, &applicationsetpkg.ApplicationSetGetQuery{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("getting applicationset %q: %w", name, err)
	}
	return appSet, nil
}

func (c *Client) DeleteApplicationSet(ctx context.Context, name string) error {
	client, closer, err := c.newAppSetClient()
	if err != nil {
		return err
	}
	defer closer()

	_, err = client.Delete(ctx, &applicationsetpkg.ApplicationSetDeleteRequest{
		Name: name,
	})
	if err != nil {
		return fmt.Errorf("deleting applicationset %q: %w", name, err)
	}
	return nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd sikifanso && go build ./internal/argocd/grpcclient/`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add internal/argocd/grpcclient/
git commit -m "feat(grpcclient): add ApplicationSetService methods"
```

---

### Task 22: Add ProjectService to gRPC Client

**Files:**
- Create: `internal/argocd/grpcclient/projects.go`

- [ ] **Step 1: Define types**

Append to `internal/argocd/grpcclient/types.go`:

```go
type ProjectSummary struct {
	Name         string
	Description  string
	Destinations []string
	Sources      []string
}

type ProjectSpec struct {
	Name         string
	Description  string
	Destinations []ProjectDestination
	Sources      []string
	Resources    []string // allowed resource kinds, e.g. "Deployment", "Service"
}

type ProjectDestination struct {
	Server    string
	Namespace string
}
```

- [ ] **Step 2: Implement Project methods**

```go
// internal/argocd/grpcclient/projects.go
package grpcclient

import (
	"context"
	"fmt"

	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) ListProjects(ctx context.Context) ([]ProjectSummary, error) {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	list, err := client.List(ctx, &projectpkg.ProjectQuery{})
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	result := make([]ProjectSummary, 0, len(list.Items))
	for _, item := range list.Items {
		ps := ProjectSummary{
			Name:        item.Name,
			Description: item.Spec.Description,
		}
		for _, d := range item.Spec.Destinations {
			ps.Destinations = append(ps.Destinations, fmt.Sprintf("%s/%s", d.Server, d.Namespace))
		}
		for _, s := range item.Spec.SourceRepos {
			ps.Sources = append(ps.Sources, s)
		}
		result = append(result, ps)
	}
	return result, nil
}

func (c *Client) GetProject(ctx context.Context, name string) (*ProjectSummary, error) {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return nil, err
	}
	defer closer()

	proj, err := client.Get(ctx, &projectpkg.ProjectQuery{Name: name})
	if err != nil {
		return nil, fmt.Errorf("getting project %q: %w", name, err)
	}

	ps := &ProjectSummary{
		Name:        proj.Name,
		Description: proj.Spec.Description,
	}
	for _, d := range proj.Spec.Destinations {
		ps.Destinations = append(ps.Destinations, fmt.Sprintf("%s/%s", d.Server, d.Namespace))
	}
	for _, s := range proj.Spec.SourceRepos {
		ps.Sources = append(ps.Sources, s)
	}
	return ps, nil
}

func (c *Client) CreateProject(ctx context.Context, spec ProjectSpec) error {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return err
	}
	defer closer()

	destinations := make([]v1alpha1.ApplicationDestination, 0, len(spec.Destinations))
	for _, d := range spec.Destinations {
		destinations = append(destinations, v1alpha1.ApplicationDestination{
			Server:    d.Server,
			Namespace: d.Namespace,
		})
	}

	_, err = client.Create(ctx, &projectpkg.ProjectCreateRequest{
		Project: &v1alpha1.AppProject{
			ObjectMeta: metav1.ObjectMeta{Name: spec.Name},
			Spec: v1alpha1.AppProjectSpec{
				Description:  spec.Description,
				Destinations: destinations,
				SourceRepos:  spec.Sources,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("creating project %q: %w", spec.Name, err)
	}
	return nil
}

func (c *Client) DeleteProject(ctx context.Context, name string) error {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return err
	}
	defer closer()

	_, err = client.Delete(ctx, &projectpkg.ProjectQuery{Name: name})
	if err != nil {
		return fmt.Errorf("deleting project %q: %w", name, err)
	}
	return nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd sikifanso && go build ./internal/argocd/grpcclient/`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add internal/argocd/grpcclient/
git commit -m "feat(grpcclient): add ProjectService methods"
```

---

### Task 23: Add `argocd projects` CLI Commands

**Files:**
- Create: `cmd/sikifanso/argocd_projects.go`
- Modify: `cmd/sikifanso/argocd_cmd.go`

- [ ] **Step 1: Create argocd projects commands**

```go
// cmd/sikifanso/argocd_projects.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdProjectsCmd() *cli.Command {
	return &cli.Command{
		Name:  "projects",
		Usage: "Manage ArgoCD AppProjects",
		Commands: []*cli.Command{
			argocdProjectsListCmd(),
			argocdProjectsCreateCmd(),
			argocdProjectsDeleteCmd(),
		},
	}
}

func argocdProjectsListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all ArgoCD projects",
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			projects, err := client.ListProjects(ctx)
			if err != nil {
				return err
			}

			if outputJSON(cmd, projects) {
				return nil
			}

			for _, p := range projects {
				fmt.Fprintf(os.Stderr, "  %-20s %s\n", p.Name, p.Description)
				if len(p.Destinations) > 0 {
					fmt.Fprintf(os.Stderr, "    destinations: %s\n", strings.Join(p.Destinations, ", "))
				}
			}
			return nil
		}),
	}
}

func argocdProjectsCreateCmd() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create an ArgoCD project",
		ArgsUsage: "NAME",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "description", Usage: "Project description"},
			&cli.StringSliceFlag{Name: "destination", Usage: "Allowed destination (server/namespace), repeatable"},
			&cli.StringSliceFlag{Name: "source", Usage: "Allowed source repo URL, repeatable"},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("project name required")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			destinations := make([]grpcclient.ProjectDestination, 0)
			for _, d := range cmd.StringSlice("destination") {
				parts := strings.SplitN(d, "/", 2)
				dest := grpcclient.ProjectDestination{Server: parts[0]}
				if len(parts) > 1 {
					dest.Namespace = parts[1]
				}
				destinations = append(destinations, dest)
			}

			spec := grpcclient.ProjectSpec{
				Name:         name,
				Description:  cmd.String("description"),
				Destinations: destinations,
				Sources:      cmd.StringSlice("source"),
			}

			if err := client.CreateProject(ctx, spec); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Project %q created\n", name)
			return nil
		}),
	}
}

func argocdProjectsDeleteCmd() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Delete an ArgoCD project",
		ArgsUsage: "NAME",
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("project name required")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			if err := client.DeleteProject(ctx, name); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Project %q deleted\n", name)
			return nil
		}),
	}
}
```

- [ ] **Step 2: Register in argocdCmd**

Add `argocdProjectsCmd()` to the Commands slice in `argocd_cmd.go`.

- [ ] **Step 3: Verify and commit**

Run: `cd sikifanso && go build ./cmd/sikifanso`

```bash
git add cmd/sikifanso/argocd_projects.go cmd/sikifanso/argocd_cmd.go
git commit -m "feat(cli): add argocd projects list/create/delete commands"
```

---

### Task 24: Add MCP Tools for Projects

**Files:**
- Modify: `internal/mcp/argocd.go`

- [ ] **Step 1: Add project MCP tools**

Append to the `registerArgoCDTools` function in `internal/mcp/argocd.go`:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_projects_list",
		Description: "List all ArgoCD AppProjects",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdAppsInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if e != nil {
			return r, sv, e
		}

		client, err := grpcclient.NewClient(ctx, grpcclient.Options{
			Address:  sess.Services.ArgoCD.GRPCAddress,
			Username: sess.Services.ArgoCD.Username,
			Password: sess.Services.ArgoCD.Password,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("gRPC client: %w", err)
		}
		defer client.Close()

		projects, err := client.ListProjects(ctx)
		if err != nil {
			return nil, nil, err
		}

		var sb strings.Builder
		for _, p := range projects {
			fmt.Fprintf(&sb, "%-20s %s\n", p.Name, p.Description)
			if len(p.Destinations) > 0 {
				fmt.Fprintf(&sb, "  destinations: %s\n", strings.Join(p.Destinations, ", "))
			}
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_project_detail",
		Description: "Get details of an ArgoCD AppProject",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdAppInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if e != nil {
			return r, sv, e
		}

		client, err := grpcclient.NewClient(ctx, grpcclient.Options{
			Address:  sess.Services.ArgoCD.GRPCAddress,
			Username: sess.Services.ArgoCD.Username,
			Password: sess.Services.ArgoCD.Password,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("gRPC client: %w", err)
		}
		defer client.Close()

		proj, err := client.GetProject(ctx, input.Name)
		if err != nil {
			return nil, nil, err
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Project: %s\nDescription: %s\n", proj.Name, proj.Description)
		if len(proj.Destinations) > 0 {
			fmt.Fprintf(&sb, "Destinations: %s\n", strings.Join(proj.Destinations, ", "))
		}
		if len(proj.Sources) > 0 {
			fmt.Fprintf(&sb, "Sources: %s\n", strings.Join(proj.Sources, ", "))
		}
		return textResult(sb.String())
	})
```

- [ ] **Step 2: Verify and commit**

Run: `cd sikifanso && go build ./cmd/sikifanso`

```bash
git add internal/mcp/argocd.go
git commit -m "feat(mcp): add argocd_projects_list and argocd_project_detail tools"
```

---

## Phase 5: Cleanup

### Task 25: Delete REST Client

**Files:**
- Delete: `internal/argocd/client.go`
- Delete: `internal/argocd/client_test.go`
- Modify: `internal/argocd/sync.go` — remove REST-dependent code
- Modify: `internal/mcp/doctor.go` — remove REST client usage from argocd_apps tool

- [ ] **Step 1: Update argocd_apps MCP tool to use gRPC**

In `internal/mcp/doctor.go`, rewrite the `argocd_apps` tool to use gRPC client instead of the REST client:

```go
// Replace REST client creation with:
client, err := grpcclient.NewClient(ctx, grpcclient.Options{
	Address:  sess.Services.ArgoCD.GRPCAddress,
	Username: sess.Services.ArgoCD.Username,
	Password: sess.Services.ArgoCD.Password,
})
if err != nil {
	return nil, nil, fmt.Errorf("gRPC client: %w", err)
}
defer client.Close()

apps, err := client.ListApplications(ctx)
```

Adapt the output formatter to use `grpcclient.AppStatus` instead of `argocd.AppStatus`.

- [ ] **Step 2: Remove REST-dependent sync functions**

In `internal/argocd/sync.go`, remove:
- `syncSingleApp()` — replaced by gRPC orchestrator
- `setupPoll()` — replaced by gRPC Watch
- `syncAndWait()` — replaced by gRPC orchestrator
- `waitForApps()` — replaced by gRPC Watch
- Any remaining references to the REST `Client` type

Keep only utility functions that are still needed (e.g., constants like `DefaultSyncTimeout` if referenced elsewhere, status color helpers).

- [ ] **Step 3: Remove status.go REST dependencies**

In `internal/argocd/status.go`, `SyncAndReport` and `ReportStatus` use the REST client. These are now replaced by the gRPC sync orchestrator + `printSyncResults` in middleware.go. Delete `SyncAndReport` and `ReportStatus`. Keep `printAppStatusTable` and `colorizeStatus` if used elsewhere, or delete if fully replaced.

- [ ] **Step 4: Delete client.go and client_test.go**

```bash
rm internal/argocd/client.go internal/argocd/client_test.go
```

- [ ] **Step 5: Fix all compilation errors**

Run: `cd sikifanso && go build ./cmd/sikifanso 2>&1`

Fix any remaining references to the deleted REST client. Common locations:
- `internal/argocd/sync.go` — remove unused imports
- `internal/mcp/helpers.go` — update `triggerSync` if it still references REST
- `cmd/sikifanso/middleware.go` — remove REST fallback path

- [ ] **Step 6: Run full test suite**

Run: `cd sikifanso && go test ./... -race`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor(argocd): remove REST client, complete gRPC migration"
```

---

### Task 26: Final Cleanup and Lint

**Files:**
- Various — fix any lint issues

- [ ] **Step 1: Run linter**

Run: `cd sikifanso && make lint`
Expected: Clean or fix any issues

- [ ] **Step 2: Run full test suite one more time**

Run: `cd sikifanso && make test`
Expected: All PASS

- [ ] **Step 3: Verify build**

Run: `cd sikifanso && make build`
Expected: Build succeeds

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "chore: fix lint issues after gRPC migration"
```

---

## Summary

| Phase | Tasks | Key Outcome |
|-------|-------|-------------|
| 1: Infrastructure + Client | 1-9 | gRPC port exposed, client package with full ApplicationService |
| 2: Sync Rewrite | 10-14 | Robust sync with watch-based waiting, webhook deletion |
| 3: Observability | 15-20 | Resource tree in doctor, new CLI commands, new MCP tools |
| 4: AppSet + Projects | 21-24 | ApplicationSet and Project management via gRPC |
| 5: Cleanup | 25-26 | REST client deleted, full gRPC end state |
