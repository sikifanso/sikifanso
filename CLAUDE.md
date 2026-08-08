# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Go CLI that bootstraps k3d Kubernetes clusters purpose-built for AI agent infrastructure: Cilium CNI, ArgoCD GitOps reading a local filesystem repo, a curated app catalog (entries defined in the `sikifanso-homelab-bootstrap` repo, so the set changes independently of this codebase), and isolated agent sandboxes.

## Build & Test

```bash
make build          # go build -o bin/sikifanso ./cmd/sikifanso
make test           # go test -race -count=1 ./...
make lint           # golangci-lint run ./...   (CI enforces this — run before committing)
make fmt            # goimports -w .  (lint also enforces gofumpt)
make snapshot       # goreleaser build --snapshot --clean

# Single test
go test ./internal/catalog/ -run TestFind -race
```

Go 1.26. Module path is `github.com/alicanalbayrak/sikifanso` (the GitHub remote is `sikifanso/sikifanso` — the module path intentionally differs). CI runs golangci-lint → `go build ./...` → `go test ./... -race`.

Note: the checked-in `.mcp.json` runs `./bin/sikifanso`, so `make build` is what refreshes the MCP server — run it after changing anything the MCP tools reach, then restart the server.

It used to run `sikifanso` from PATH, which silently served whatever release happened to be installed: on a machine with a Homebrew build on PATH the MCP tools drove that binary, not the working tree, with no error to say so. `go install ./cmd/sikifanso` does not fix that unless `$(go env GOPATH)/bin` precedes the release on your PATH — check with `which -a sikifanso` before trusting it.

## Architecture

**CLI framework**: `urfave/cli/v3`. Single binary package `cmd/sikifanso/`. Root command in `app.go`; `setup.go` has a `Before` hook for zap logger init (so `--help` works without Docker). Visible command groups: `cluster`, `app`, `agent`, `snapshot` (+ hidden `mcp serve`).

**Command middleware** (`cmd/sikifanso/middleware.go`):
- `wrapAction` — timing/logging around any action
- `withSession` — loads the cluster session, passes it to the handler
- `syncAfterMutation` — post-mutation ArgoCD sync (enable/disable/add/remove). **Non-fatal**: if the gRPC client can't be built it warns and returns nil — the gitops commit has already happened, so a successful command does not guarantee a deployed app
- `rejectPositionalArgs` — cluster targeting is `--cluster/-c` only; positional names are rejected

**Cluster creation flow** (`internal/cluster/cluster.go`): resolve ports → remove stale session dir → scaffold gitops repo → create k3d cluster (gitops dir hostPath-mounted at `/local-gitops` on all nodes) → install Cilium → install ArgoCD → `WaitForGRPC` → imperatively create `cilium`/`argocd` Application CRDs → apply `root-app.yaml` → wait for infra healthy → apply `root-catalog.yaml` + `root-agents.yaml`. k3s runs with flannel, kube-proxy, network-policy, traefik, and servicelb disabled — **Cilium is mandatory**; a cluster without it has no networking.

**Triple-track app model** — three ApplicationSets in the bootstrap repo generate Applications from gitops files:

| AppSet CR name | Manifest | Watches | Written by |
|---|---|---|---|
| `root` | `bootstrap/root-app.yaml` | `apps/coordinates/*.yaml` | `app add/remove` (custom charts) |
| `catalog` | `bootstrap/root-catalog.yaml` | `catalog/*.yaml` where `enabled: true` | `app enable/disable`, profiles, TUI |
| `agents` | `bootstrap/root-agents.yaml` | `agents/*.yaml` | `agent create/delete` |

**ArgoCD sync — two mechanisms** (`internal/argocd/`):
- `appsetreconcile` — patches the `application-set-refresh` annotation on an ApplicationSet CR to force immediate reconciliation. Used when Application CRs must appear/disappear (enable/disable, agent ops, MCP, dashboard).
- `grpcsync` — orchestrator over the ArgoCD gRPC API (`grpcclient` wraps `argo-cd/v3/pkg/apiclient`). Watches per-app with poll fallback, 60s Degraded grace period, and tier sequencing: tiers sort lexically (`0-operators` < `1-data`), reversed on disable; falls back silently to concurrent sync if no requested app has a tier.
- Enable vs Sync matters: after enabling, Application CRs don't exist yet, so the flow must use `OpEnable` (annotate → poll for CR → watch), not `OpSync` (see comment in `cluster_create.go`).

**GitOps is local**: no remote git server. ArgoCD's repo-server reads `/local-gitops` directly (configured in `internal/infraconfig/defaults/argocd-values.yaml`). Changes become visible to ArgoCD only via `gitops.Commit` (go-git, `internal/gitops/`). `catalog.SetEnabled` writes the file but **never commits** — callers (`catalog.Toggle`, `profile.Apply`, TUI) must commit themselves.

**Session state** (`internal/session/`): YAML at `~/.sikifanso/clusters/<name>/session.yaml` (credentials, ports, service URLs). Root dir overridable via `SIKIFANSO_HOME` (`internal/paths/`) — this is the standard test isolation hook.

**Infrastructure config** (`internal/infraconfig/`): embedded defaults (`//go:embed defaults/*.yaml`) deep-merged with optional user overrides at `gitops/infra/*.yaml`.

**Catalog** (`internal/catalog/`): entries in `gitops/catalog/*.yaml` with `tier` and `dependsOn` fields. `SetEnabled` edits the yaml.v3 AST to preserve comments/order — the only AST-edit path in the repo. `depgraph.go` resolves transitive deps (enable cascades; disable refuses if dependents exist, `--force` bypasses but does not cascade).

**MCP server** (`internal/mcp/`): 25 tools over stdio via `modelcontextprotocol/go-sdk`, reusing the same internal packages as the CLI.

**Other packages**: `doctor` (pluggable `Check` interface, three tiers: infra/cluster/app), `profile` (compiled-in presets enabling catalog app sets), `snapshot` (tar.gz of session + gitops incl. `.git`), `tui` (Bubble Tea catalog browser), `dashboard` (embedded net/http status UI), `upgrade` (Helm upgrade with pre-snapshot and auto-rollback), `helm`/`cilium`/`kube` (SDK wrappers).

## Key Patterns

- **YAML**: `sigs.k8s.io/yaml` for struct marshal/unmarshal — structs use `json:` tags (exception: `infraconfig` structs have inert `yaml:` tags and rely on case-insensitive field matching). `gopkg.in/yaml.v3` only for the catalog AST edit.
- **Logging**: one zap logger created in the root `Before` hook and passed explicitly as `*zap.Logger` params — no package-level loggers in `internal/`. Dual sink: colored console (stderr) + always-Debug JSON file with rotation. k3d's logrus is redirected into zap (`internal/logger/`).
- **Output convention**: all human-facing UI (tables, spinners, prompts) goes to **stderr**; only `--output json` and `app logs` write to stdout.
- **Port resolution** (`internal/cluster/ports.go`): all-or-nothing — tries the six default host ports; if any is taken, allocates six ephemeral ports instead. Container-side NodePorts are fixed.

## Contract Tests

Update these when changing the corresponding surface — they assert exact lists:
- `cmd/sikifanso/command_structure_test.go` — the exact command tree (groups, subcommands, hidden `mcp`)
- `internal/mcp/server_test.go` — the exact 25-tool name list

Test conventions: in-package tests, `t.Setenv("SIKIFANSO_HOME", t.TempDir())` for isolation, fixtures built in `t.TempDir()` (no `testdata/` dirs), `internal/gitops` tests shell out to the real `git` binary.

## Cross-Repo Coupling (CLI ↔ sikifanso-homelab-bootstrap)

The bootstrap template repo is cloned during `cluster create` (`internal/gitops/scaffold.go` strips its `.git` and re-inits). Invariants nothing validates at runtime — a mismatch silently degrades to ArgoCD's 180s reconciliation interval:
- Manifests `bootstrap/root-{app,catalog,agents}.yaml` must exist (hardcoded in `internal/gitops/rootapp.go`)
- ApplicationSet CR names must be exactly `root`, `catalog`, `agents` in namespace `argocd`
- File layouts written by the CLI must match the AppSet generators' globs; catalog entry fields must match `catalog.Entry` json tags
- **Releases**: a tagged CLI release clones the bootstrap repo at the same tag (`resolveBootstrapVersion` in `cluster_create.go`) — every CLI tag needs a matching bootstrap-repo tag. Dev/snapshot builds use HEAD.
- `internal/agent/` pins the `sikifanso-agent-template` chart at a hardcoded version

## Known Doc Drift

Trust code over `docs/`: the docs still describe a dual-track (two-AppSet) model and a webhook-based sync that no longer exists, and document positional cluster-name args that `rejectPositionalArgs` now rejects.

## Commit Messages

Do not add `Co-Authored-By` or bot signature lines to commit messages.
