# ArgoCD gRPC Integration Design

**Date:** 2026-03-29
**Status:** Approved
**Author:** Alican + Claude

## Problem

The current ArgoCD integration uses a thin REST HTTP client and webhook-based sync triggering. This has three concrete failure modes:

1. **Silent progress** — `catalog enable/disable` fires a webhook and returns. The user assumes success, but sync may still be progressing or may fail later.
2. **Partial failures** — some resources apply while others fail (CRD ordering, dependency issues). The CLI reports success because it never checked.
3. **Race conditions** — rapid enable/disable operations cause ArgoCD to reconcile in unexpected order, leading to inconsistent state.

Beyond sync robustness, the REST client limits observability (no resource-level health), lifecycle operations (no rollback, no delete), and future capabilities (no project management, no streaming).

## Decision

Adopt ArgoCD's gRPC API via the `github.com/argoproj/argo-cd/v2` SDK. Migrate incrementally from REST to gRPC (Approach B: Layer), with the end state being full gRPC and REST client deletion.

**Services in scope:**
- ApplicationService — sync, watch, resource tree, managed resources, rollback, delete, logs, actions
- ApplicationSetService — CRUD for the dual-track ApplicationSets
- ProjectService — programmatic AppProject management (mechanics only, policy TBD)

## Section 1: Infrastructure — Exposing gRPC from ArgoCD

ArgoCD multiplexes HTTP and gRPC on the same port when TLS is enabled. Since sikifanso runs in insecure mode (no TLS), a **separate gRPC port** is required.

### Changes

**`internal/infraconfig/defaults/argocd-values.yaml`** — add gRPC service configuration:
- `server.service.nodePortGrpc` — dedicated NodePort for gRPC
- gRPC service type: NodePort (matching existing HTTP service)

**`internal/cluster/ports.go`** — add gRPC port to default/fallback allocation:
- New default port alongside existing ArgoCD HTTP port
- Same fallback-to-free-port logic for multi-cluster support

**`internal/session/session.go`** — extend `ArgoCDInfo`:
- Add `GRPCAddress string` field (e.g., `localhost:30081`)
- Populated during cluster creation after port resolution

**Cluster creation flow** — store gRPC address in session after ArgoCD install completes.

### Why separate port over gRPC-Web
- Native gRPC performance — no proxy overhead, critical for streaming
- Port resolution system already handles multi-port allocation
- Clean separation: HTTP for browser/legacy access, gRPC for CLI/MCP

## Section 2: gRPC Client Package

**New package: `internal/argocd/grpcclient/`**

Separate from the existing REST client. Wraps ArgoCD SDK's typed gRPC stubs into a focused client tailored to sikifanso's needs.

### Package Structure

```
internal/argocd/grpcclient/
├── client.go           # Connection management, auth, lifecycle
├── applications.go     # ApplicationService operations
├── applicationsets.go  # ApplicationSetService operations
├── projects.go         # ProjectService operations
```

### client.go — Connection & Auth

- Accepts `GRPCAddress` and credentials from session
- Establishes insecure gRPC connection (local k3d, no TLS)
- Authenticates via `SessionService.Create()` to get JWT token
- Attaches token as gRPC metadata via per-call auth interceptor
- Exposes `Close()` for clean shutdown
- Connection created on-demand, not at CLI startup

### applications.go — ApplicationService

| Method | ArgoCD RPC | Purpose |
|---|---|---|
| `List()` | `List` | List all apps with sync/health status |
| `Get(name)` | `Get` | Single app details |
| `Sync(name, opts)` | `Sync` | Trigger sync with prune option |
| `Watch(name)` | `Watch` | Stream real-time status changes (returns channel) |
| `ResourceTree(name)` | `ResourceTree` | Full resource hierarchy |
| `ManagedResources(name)` | `ManagedResources` | Live vs desired state diff |
| `Rollback(name, id)` | `Rollback` | Rollback to previous sync revision |
| `Delete(name, cascade)` | `Delete` | Delete app with cascade option |
| `PodLogs(name, pod, container)` | `PodLogs` | Stream container logs |
| `RunAction(name, resource, action)` | `RunResourceAction` | Resource actions (restart, retry) |

### applicationsets.go — ApplicationSetService

| Method | ArgoCD RPC | Purpose |
|---|---|---|
| `List()` | `List` | List all ApplicationSets |
| `Get(name)` | `Get` | Get ApplicationSet details |
| `Create(spec)` | `Create` | Create new ApplicationSet |
| `Delete(name)` | `Delete` | Delete ApplicationSet |

### projects.go — ProjectService

| Method | ArgoCD RPC | Purpose |
|---|---|---|
| `List()` | `List` | List all AppProjects |
| `Get(name)` | `Get` | Get project details |
| `Create(spec)` | `Create` | Create AppProject |
| `Update(spec)` | `Update` | Update project config |
| `Delete(name)` | `Delete` | Delete project |

### Design Decisions

- **Domain type translation** — methods return sikifanso domain types, not raw ArgoCD proto types. The gRPC layer translates. This prevents proto type leakage into the rest of the codebase.
- **Streaming via channels** — `Watch()` and `PodLogs()` return Go channels, handling gRPC stream lifecycle internally.
- **Context propagation** — all methods accept `context.Context` for timeout/cancellation.

## Section 3: Robust Sync — Replacing Fire-and-Forget

### Current Flow (broken)
```
catalog enable foo → git commit → webhook fire → CLI exits → ???
```

### New Flow
```
catalog enable foo → git commit → gRPC Sync(foo) → Watch(foo) stream → block until Synced+Healthy or timeout → exit 0/1
```

### Sync Orchestrator

Replaces the existing `SyncMode` enum and webhook logic in `internal/argocd/sync.go`.

```go
type SyncRequest struct {
    Apps           []string       // target apps (empty = all)
    Timeout        time.Duration  // default 2m, configurable via --timeout
    Prune          bool           // remove resources not in git (default true)
    SkipUnhealthy  bool           // don't fail on pre-existing degraded apps
}

type SyncResult struct {
    App        string
    SyncStatus string            // Synced, OutOfSync
    Health     string            // Healthy, Degraded, Progressing, etc.
    Message    string            // human-readable failure reason
    Resources  []ResourceStatus  // per-resource breakdown on failure
}
```

### How It Works

1. **Trigger** — call `gRPC Sync(app)` with prune option. Replaces webhook broadcast with explicit, targeted sync instruction.

2. **Stream** — open `Watch(app)` stream immediately after sync trigger. Receive real-time status events.

3. **Progress reporting** — live CLI output as events arrive:
   ```
   foo: OutOfSync → Progressing → Synced/Healthy ✓   (12s)
   ```

4. **Failure detection** — if health goes `Degraded` or sync stays `OutOfSync`, fetch `ResourceTree(app)` for per-resource breakdown:
   ```
   foo: Synced/Degraded ✗  (45s)
     └─ Deployment/foo-server: CrashLoopBackOff (exit code 1)
   ```

5. **Timeout** — if deadline elapses, collect current state, report, exit non-zero.

6. **Multi-app** — for `profile apply` and similar, watch all apps concurrently. Report each as it resolves. Exit when all done or timeout.

### Webhook Deletion

All webhook logic is deleted:
- `sendServerWebhook()` — deleted
- `sendAppSetWebhook()` — deleted
- GitHub push event emulation — deleted
- Kubernetes API server proxy for ApplicationSet controller — deleted

gRPC `Sync()` is a direct, authenticated instruction — no need to emulate external events.

### Integration Points

| Command | Current | New |
|---|---|---|
| `catalog enable/disable` | `syncAfterMutation` → webhook | `syncAfterMutation` → gRPC sync+watch |
| `catalog enable --no-wait` | N/A | gRPC sync trigger only |
| `app add/remove` | webhook | gRPC sync+watch |
| `profile apply` | webhook | gRPC multi-app sync+watch |
| `argocd sync` | webhook or REST | gRPC sync+watch |
| `argocd sync --no-wait` | SyncModeFire | gRPC sync trigger only |

### CLI Flags

- `--no-wait` — trigger sync but don't block (replaces old `SyncModeFire`)
- `--timeout <duration>` — sync wait timeout (default 2m)
- `--skip-unhealthy` — don't fail on pre-existing degraded apps

### Exit Codes

- `0` — all apps synced and healthy
- `1` — at least one app failed (degraded, sync error)
- `2` — timeout reached, status unknown/progressing

## Section 4: Enhanced Observability

### 4a. Resource-Level Health in Doctor

`internal/doctor/argocd.go` enhancement:

- After deployment availability check, call `List()` for all app statuses
- For unhealthy apps, call `ResourceTree(app)` to identify failing resources
- Output changes from:
  ```
  ✗ catalog-app "langfuse" is Degraded
  ```
  to:
  ```
  ✗ catalog-app "langfuse" is Degraded
    └─ StatefulSet/langfuse-clickhouse: 0/1 replicas ready
    └─ PVC/data-langfuse-clickhouse-0: Pending (no matching StorageClass)
  ```

### 4b. MCP Tools

| Tool | Description |
|---|---|
| `argocd_apps` (enhanced) | List apps + resource tree summary for unhealthy apps |
| `argocd_app_detail` (new) | Full resource tree, managed resources, diff for single app |
| `argocd_app_logs` (new) | Stream pod logs for an app |
| `argocd_app_diff` (new) | Live vs desired state diff |
| `argocd_rollback` (new) | Rollback app to previous revision |
| `argocd_projects_list` (new) | List AppProjects |
| `argocd_project_detail` (new) | Project details (destinations, sources, resource whitelist) |

### 4c. New CLI Commands

```
sikifanso argocd status [app]                              # detailed status with resource tree
sikifanso argocd diff <app>                                # live vs desired state
sikifanso argocd logs <app> [--pod] [--container] [--follow]
sikifanso argocd rollback <app> [--revision]
sikifanso argocd projects list
sikifanso argocd projects create <name> [--destinations] [--sources]
sikifanso argocd projects delete <name>
```

### 4d. Deletions After Migration

- `internal/argocd/client.go` — REST client deleted
- `internal/argocd/client_test.go` — replaced with gRPC client tests
- Webhook logic in `sync.go` — deleted
- `sendAppSetWebhook()` and k8s proxy path — deleted

## Section 5: ProjectService and Migration Strategy

### 5a. ProjectService — Mechanics Layer

Exposes AppProject CRUD via gRPC client and CLI. No fixed policy — the capability is available for future use.

**Potential future wiring** (not part of this design):
- `agent add` could create a scoped AppProject per agent namespace
- Profiles could define project boundaries
- MCP consumers could inspect project RBAC

### 5b. Migration Phases

Each phase is independently shippable. Each follows: spec → agent review → implement → independent verify → `/simplify`.

**Phase 1: Infrastructure + gRPC Client**
- Expose gRPC port from ArgoCD (Helm values + port allocation)
- Add `GRPCAddress` to session
- Implement `internal/argocd/grpcclient/` with ApplicationService methods
- Existing REST client untouched

**Phase 2: Sync Rewrite**
- Replace `syncAfterMutation` middleware to use gRPC sync+watch
- Add `--no-wait` and `--timeout` flags
- Real-time progress output
- Resource-level failure reporting
- Delete webhook logic

**Phase 3: Observability**
- Enrich `doctor` with resource tree details
- Add CLI commands: `status`, `diff`, `logs`, `rollback`
- Add new MCP tools
- Migrate `argocd_apps` MCP tool to gRPC

**Phase 4: ApplicationSet + Project Services**
- Add ApplicationSetService methods to gRPC client
- Add ProjectService methods to gRPC client
- Add `argocd projects *` CLI commands
- Add MCP tools for projects

**Phase 5: Cleanup**
- Delete `internal/argocd/client.go` (REST)
- Delete `internal/argocd/client_test.go`
- Remove REST-related helpers
- Update all tests to use gRPC client

## Development Workflow

Spec-driven with agent fleet:
1. **Spec agents** write detailed per-phase specs
2. **Review agents** review specs for completeness and correctness
3. **Implementor agents** implement against the spec
4. **Verifier agents** independently check implementation against spec
5. **Between steps:** run `/simplify` for code quality
