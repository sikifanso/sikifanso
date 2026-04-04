# Catalog App Sync Ordering via ArgoCD RollingSync + Dependency Graph

**Date:** 2026-04-04
**Status:** Approved

## Problem

When a profile enables multiple catalog apps (e.g. cnpg-operator + postgresql), ArgoCD syncs them all in parallel. If app B depends on CRDs from app A, app B fails because A hasn't installed its CRDs yet.

Concrete failures:
1. `postgresql` syncs before `cnpg-operator` finishes — `Cluster` CRD not found (`postgresql.cnpg.io/Cluster`)
2. CNPG cluster chart with `monitoring.enabled: true` produces `PrometheusRule`/`PodMonitor` resources requiring `monitoring.coreos.com` CRDs from `prometheus-stack`

Root cause: `root-catalog.yaml` ApplicationSet uses `syncPolicy.applicationsSync: sync` which syncs all generated Applications in parallel with no ordering.

## Solution

Two complementary mechanisms:

1. **Server-side:** ArgoCD ApplicationSet RollingSync strategy groups apps into ordered tiers
2. **CLI-side:** Dependency graph enables auto-enable on `app enable` and blocks unsafe `app disable`

## Tier-Based RollingSync (ArgoCD Side)

### How RollingSync Works

- The ApplicationSet controller takes over sync triggering (replaces `syncPolicy.automated` on individual apps)
- Apps are grouped by label (e.g. `tier: "0-operators"`)
- Steps are processed sequentially — each step waits for all its apps to be Healthy before proceeding
- Drift detection still works: if an app goes OutOfSync and its tier's prerequisites are healthy, the controller re-syncs it automatically
- Must be explicitly enabled on the ArgoCD server with `--enable-progressive-syncs` flag
- `syncPolicy.automated` (including `selfHeal`) is forcibly disabled by the controller — the controller handles all sync triggering
- Apps not matching any step are synced last (after all defined steps complete)
- `retry` settings in `syncPolicy` are still respected when syncs are triggered

### Tier Assignments

| Tier | Label | Apps |
|------|-------|------|
| 0 | `0-operators` | cnpg-operator, prometheus-stack, external-secrets |
| 1 | `1-data` | postgresql |
| 2 | `2-services` | langfuse, temporal |
| _(none)_ | _(unmatched)_ | All independent apps — synced after all defined steps (ArgoCD native) |

Tier naming convention sorts lexicographically: `0-operators` < `1-data` < `2-services`.

Independent apps (litellm-proxy, ollama, qdrant, text-embeddings-inference, unstructured, guardrails-ai, nemo-guardrails, presidio, valkey, loki, tempo, alloy, opa) have no `tier` field. ArgoCD's RollingSync syncs unmatched apps after all defined steps complete.

### root-catalog.yaml Changes

- Replace `applicationsSync: sync` with `strategy.type: RollingSync` and step definitions matching each tier label
- Add `tier: "{{tier}}"` label to the template metadata
- Remove `syncPolicy.automated` (RollingSync takes over sync triggering)
- Add `SkipDryRunOnMissingResource=true` to syncOptions
- Add retry policy for resilience
- Keep existing syncOptions: `CreateNamespace=true`, `ServerSideApply=true`, `RespectIgnoreDifferences=true`

### ArgoCD Config

Add `--enable-progressive-syncs` to the ApplicationSet controller args in `internal/infraconfig/defaults/argocd-values.yaml`.

## Catalog Entry Schema Changes

File: `internal/catalog/catalog.go`

Add two fields to `Entry` (both `omitempty` for backward compatibility):

```go
Tier      string   `json:"tier,omitempty"`
DependsOn []string `json:"dependsOn,omitempty"`
```

Entries without `tier` default to ArgoCD's unmatched behavior (synced last). Entries without `dependsOn` have no CLI-level dependency constraints.

### Dependency Declarations

| App | dependsOn |
|-----|-----------|
| postgresql | cnpg-operator, prometheus-stack |
| langfuse | cnpg-operator, postgresql |
| temporal | cnpg-operator, postgresql |
| All others | _(none)_ |

## Dependency Graph

New file: `internal/catalog/depgraph.go`

Three functions:

- **`ResolveDeps(requested []string, all []Entry) (resolved, autoAdded []string, err error)`** — BFS transitive resolution. Returns full ordered set + which ones were auto-added. Includes cycle detection.
- **`Dependents(name string, all []Entry) []string`** — Reverse lookup: which enabled apps depend on this app?
- **Cycle detection** — Returns error if the dependency graph contains cycles.

New file: `internal/catalog/depgraph_test.go` — Tests for DAG resolution, cycle detection, transitive deps, and reverse lookup.

## ToggleWithDeps

File: `internal/catalog/service.go`

New function alongside existing `Toggle`:

### Enable Path
1. Load all catalog entries
2. Call `ResolveDeps` with the requested app
3. Auto-enable any missing dependencies
4. Commit all changes (deps + requested app)
5. Return `ToggleWithDepsResult` including auto-added dep names

### Disable Path
1. Load all catalog entries
2. Call `Dependents` to find enabled apps that depend on this one
3. If any exist and `--force` is not set: return error with message listing dependents and hint to use `--force`
4. If `--force` or no dependents: proceed with normal disable

Existing `Toggle` and `Flip` remain unchanged (used by TUI browser).

### Disable --force Behavior

`--force` bypasses the dependent check and disables only the requested app. It does NOT cascade-disable dependents. Users must explicitly disable leaf-to-root if they want to tear down a chain.

## CLI Updates

File: `cmd/sikifanso/app_cmd.go`

- `appToggleAction` (~line 331): Use `ToggleWithDeps`, print auto-enabled deps notification ("auto-enabled: cnpg-operator, prometheus-stack")
- Add `--force` flag to `appDisableCmd`
- TUI browser flow: Unchanged (uses `Toggle`/`Flip` directly, no dep resolution)

## Profile Apply Update

File: `internal/profile/profile.go`

- Signature changes: `Apply(...) ([]string, error)` — returns list of auto-added dep names
- Internally calls `ResolveDeps` to expand profile apps with transitive deps before enabling
- Returns auto-added deps so callers can notify users

### Call Site Updates

- `cmd/sikifanso/cluster_create.go` (~line 95): Handle `(autoAdded, err)` return, log auto-added deps
- `internal/mcp/helpers.go` (~line 76): Handle `(autoAdded, err)` return, include in result text

## MCP Handler Update

File: `internal/mcp/catalog.go`

- `catalogToggle` (~line 102): Use `ToggleWithDeps` instead of `Toggle`
- Response text includes auto-added deps when enabling
- Disable with dependents returns error (no `--force` equivalent in MCP — agents should disable explicitly)

## What Doesn't Change

- `internal/argocd/grpcsync/orchestrator.go` — No wave logic needed; ArgoCD handles ordering server-side via RollingSync
- `syncAfterMutation` middleware — Continues working as-is
- `Toggle` / `Flip` — Preserved for TUI browser

## Catalog YAML Updates

All 19 entries in `sikifanso-homelab-bootstrap/catalog/*.yaml` get `tier` and `dependsOn` fields where applicable:

**Tier 0-operators:** cnpg-operator, prometheus-stack, external-secrets
**Tier 1-data:** postgresql
**Tier 2-services:** langfuse, temporal
**No tier (unmatched):** alloy, guardrails-ai, litellm-proxy, loki, nemo-guardrails, ollama, opa, presidio, qdrant, tempo, text-embeddings-inference, unstructured, valkey

## Verification

1. `make test` — all existing tests pass
2. `go test ./internal/catalog/ -run TestResolveDeps -race` — dependency resolution
3. `make lint` — no lint issues
4. End-to-end: `sikifanso cluster create --profile agent-minimal` — verify postgresql waits for cnpg-operator
5. `sikifanso app enable postgresql` — verify cnpg-operator + prometheus-stack auto-enabled
6. `sikifanso app disable cnpg-operator` — verify error listing postgresql as dependent
7. `sikifanso app disable cnpg-operator --force` — verify it proceeds without cascade
8. Verify ArgoCD ApplicationSet status shows step-based progress

## Key Files

| File | Change |
|------|--------|
| `sikifanso/internal/catalog/catalog.go` | Add `Tier` and `DependsOn` fields to `Entry` |
| `sikifanso/internal/catalog/depgraph.go` | NEW — `ResolveDeps`, `Dependents`, cycle detection |
| `sikifanso/internal/catalog/depgraph_test.go` | NEW — DAG resolution tests |
| `sikifanso/internal/catalog/service.go` | Add `ToggleWithDeps` |
| `sikifanso/cmd/sikifanso/app_cmd.go` | Use `ToggleWithDeps`, `--force` flag, notify auto-deps |
| `sikifanso/internal/profile/profile.go` | Resolve transitive deps in `Apply`, return `([]string, error)` |
| `sikifanso/cmd/sikifanso/cluster_create.go` | Handle updated `Apply` return |
| `sikifanso/internal/mcp/helpers.go` | Handle updated `Apply` return |
| `sikifanso/internal/mcp/catalog.go` | Use `ToggleWithDeps` |
| `sikifanso/internal/infraconfig/defaults/argocd-values.yaml` | Enable `--enable-progressive-syncs` |
| `sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml` | Add RollingSync strategy + tier label |
| `sikifanso-homelab-bootstrap/catalog/*.yaml` | Add `tier` and `dependsOn` fields to all 19 entries |
