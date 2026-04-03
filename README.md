<p align="center">
  <img src="assets/logo.png" alt="sikifanso" width="200">
</p>

<h1 align="center">sikifanso</h1>
<p align="center">
  <a href="https://github.com/sikifanso/sikifanso/releases/latest"><img src="https://img.shields.io/github/v/release/sikifanso/sikifanso?color=blue" alt="Release"></a>
  <a href="https://github.com/sikifanso/sikifanso/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/sikifanso/sikifanso/ci.yml?label=CI" alt="CI"></a>
  <a href="https://github.com/sikifanso/sikifanso/blob/main/LICENSE"><img src="https://img.shields.io/github/license/sikifanso/sikifanso" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/sikifanso/sikifanso"><img src="https://goreportcard.com/badge/github.com/sikifanso/sikifanso" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/sikifanso/sikifanso"><img src="https://img.shields.io/badge/go-1.25+-00ADD8?logo=go" alt="Go version"></a>
  <a href="https://sikifanso.com"><img src="https://img.shields.io/badge/docs-sikifanso.com-3F9AAE" alt="Docs"></a>
</p>
<p align="center">
  <img src="assets/demo.gif" alt="demo" width="800">
</p>


A CLI tool that bootstraps Kubernetes clusters purpose-built for running AI agents safely. Spins up a [k3d](https://k3d.io) cluster pre-configured with [Cilium](https://cilium.io) (eBPF networking, agent isolation), [ArgoCD](https://argoproj.github.io/cd/) (GitOps), and a curated catalog of AI agent infrastructure tools.

## What you get

```
sikifanso cluster create --profile agent-dev
```

- **k3d cluster** -- single-node k3s v1.29
- **Cilium** -- full kube-proxy replacement, ingress controller, Hubble UI, network isolation for agents
- **ArgoCD** -- configured to read from a local gitops repo on your filesystem
- **GitOps repo** -- scaffolded from a bootstrap template, mounted into the cluster
- **AI Agent Infrastructure Catalog** -- 17 curated tools across 7 categories:

| Category | Tools | Purpose |
|----------|-------|---------|
| **Gateway** | LiteLLM Proxy | LLM API routing, cost tracking, rate limiting |
| **Observability** | Langfuse, Prometheus+Grafana, Loki, Tempo | LLM tracing, metrics, logs, distributed tracing |
| **Guardrails** | Guardrails AI, NeMo Guardrails, Presidio | Output validation, safety rails, PII redaction |
| **RAG** | Qdrant, Text Embeddings Inference, Unstructured | Vector DB, embeddings, document parsing |
| **Runtime** | Temporal, External Secrets, OPA | Workflow orchestration, secrets, policy engine |
| **Models** | Ollama | Local LLM inference |
| **Storage** | PostgreSQL, Valkey (Redis) | Supporting data stores |

- **Profiles** -- predefined tool sets (`agent-minimal`, `agent-dev`, `agent-safe`, `agent-full`, `rag`)
- **Agent sandboxes** -- isolated namespaces with resource quotas and network policies
- **MCP server** -- expose cluster operations as tools for AI agents (Claude, Cursor, etc.)

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (running)

That's it. You do **not** need to install k3d, Helm, Cilium, ArgoCD, or any other Kubernetes tooling. sikifanso embeds everything and handles the full stack internally.

## Install

```bash
brew install --cask sikifanso/tap/sikifanso
```

Or with Go:

```bash
go install github.com/sikifanso/sikifanso/cmd/sikifanso@latest
```

Or build from source:

```bash
git clone https://github.com/sikifanso/sikifanso.git
cd sikifanso
go build -o sikifanso ./cmd/sikifanso
```

## Quick start

```bash
# Create a cluster with a profile
sikifanso cluster create --profile agent-dev

# List the AI infra catalog
sikifanso app list --all

# Enable an additional tool
sikifanso app enable tempo

# Create an isolated agent sandbox
sikifanso agent create my-agent --cpu 1 --memory 1Gi

# Check everything is healthy
sikifanso cluster doctor
```

After creation you'll see:

```
+------------------------------------------+
|           Cluster: default               |
+------------------------------------------+
| State:           running                 |
|                                          |
| ArgoCD URL:      http://localhost:30080  |
| ArgoCD User:     admin                   |
| ArgoCD Password: ********               |
|                                          |
| Hubble URL:      http://localhost:30081  |
|                                          |
| GitOps Path:     ~/.sikifanso/clusters/  |
+------------------------------------------+
```

## Deploying AI infra tools

### Using profiles

Profiles enable a curated set of tools in one step:

```bash
# Minimal: LLM gateway + tracing
sikifanso cluster create --profile agent-minimal

# Full development stack
sikifanso cluster create --profile agent-dev

# Everything with guardrails
sikifanso cluster create --profile agent-safe

# Compose profiles
sikifanso cluster create --profile agent-dev,rag
```

See `sikifanso cluster profiles` for the full list.

### Enabling individual tools

```bash
# Enable a tool from the catalog
sikifanso app enable litellm-proxy

# Disable a tool
sikifanso app disable litellm-proxy

# Browse the full catalog
sikifanso app list --all
```

### Custom Helm charts

Deploy any Helm chart using the `app add` command:

```bash
sikifanso app add podinfo \
  --repo https://stefanprodan.github.io/podinfo \
  --chart podinfo \
  --version 6.10.1 \
  --namespace podinfo
```

This writes the chart coordinates and a stub values file to your gitops repo, auto-commits, and triggers an ArgoCD sync. If you omit any flags, the CLI prompts interactively.

### Listing and removing apps

```bash
# List all installed apps (both custom and catalog)
sikifanso app list
```

```
NAME                 CHART                  VERSION    NAMESPACE    SOURCE
litellm-proxy        litellm                0.2.1      gateway      catalog
langfuse             langfuse               1.2.14     observability catalog
qdrant               qdrant                 0.13.2     rag          catalog
```

Remove a custom app:

```bash
sikifanso app remove podinfo
```

## Agent sandboxes

Create isolated namespaces for running untrusted AI agent code:

```bash
# Create a sandbox with resource limits
sikifanso agent create my-agent --cpu 1 --memory 1Gi --pods 20

# List agents
sikifanso agent list

# Delete an agent
sikifanso agent delete my-agent
```

Each sandbox gets resource quotas, Cilium network policies (default-deny egress, allowlisted data layer access), and its own service account.

## MCP server

Expose cluster operations as MCP tools for AI agents:

```bash
sikifanso mcp serve
```

22 tools across cluster management, catalog, agents, ArgoCD, Kubernetes, and health checks. Configure in Claude Code:

```json
{
  "mcpServers": {
    "sikifanso": {
      "command": "sikifanso",
      "args": ["mcp", "serve"]
    }
  }
}
```

## CLI reference

```
sikifanso [global flags] <command> [command flags]
```

### Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--cluster`, `-c` | `default` | Target cluster name |
| `--output`, `-o` | `table` | Output format (`table`, `json`) |
| `--log-level` | `info` | Console log level (debug, info, warn, error) |

The `--cluster` flag can also be set via `SIKIFANSO_CLUSTER` env var.

### Commands

| Command | Description |
|---------|-------------|
| **Cluster** | |
| `cluster create` | Create a new cluster (with optional `--profile`) |
| `cluster delete [NAME]` | Delete a cluster and clean up |
| `cluster info [NAME]` | Show cluster details (omit name to list all) |
| `cluster start [NAME]` | Start a stopped cluster |
| `cluster stop [NAME]` | Stop a running cluster |
| `cluster doctor` | Run health checks on the cluster |
| `cluster dashboard` | Start the local web dashboard |
| `cluster upgrade` | Upgrade Cilium and/or ArgoCD |
| `cluster profiles` | List available profiles |
| **Apps** | |
| `app add [NAME]` | Add a custom Helm chart to the gitops repo |
| `app list` | List installed apps (`--all` for full catalog) |
| `app remove NAME` | Remove a custom app |
| `app enable NAME` | Enable a catalog app |
| `app disable NAME` | Disable a catalog app |
| `app sync` | Trigger ArgoCD sync |
| `app status [APP]` | Show app status with resource tree |
| `app diff APP` | Show diff between live and desired state |
| `app logs APP` | Stream pod logs for an app |
| `app rollback APP` | Roll back to a previous revision |
| **Agents** | |
| `agent create NAME` | Create an isolated agent namespace |
| `agent list` | List agent namespaces |
| `agent delete NAME` | Delete an agent namespace |
| **Snapshots** | |
| `snapshot capture` | Capture cluster configuration state |
| `snapshot list` | List available snapshots |
| `snapshot restore NAME` | Restore from a snapshot |
| `snapshot delete NAME` | Delete a snapshot |
| **MCP** | |
| `mcp serve` | Start the MCP server (stdio) |

## Health checks

```bash
sikifanso cluster doctor
```

Runs a series of health checks against the cluster and prints a structured report. Checks Docker, k3d nodes, Cilium, Hubble, ArgoCD, every enabled catalog app, and agent namespaces. Exits 0 when everything is healthy, 1 when any check fails.

```
ok  Docker daemon       running (v27.0.3)
ok  k3d cluster         1/1 nodes ready
ok  Cilium              DaemonSet 1/1 ready
ok  Hubble              relay deployment ready
ok  ArgoCD              3/3 deployments ready
ok  App: litellm-proxy  Healthy -- Synced
ok  App: langfuse       Healthy -- Synced
!!  App: qdrant          Degraded -- Synced
                         -> StatefulSet qdrant in namespace rag: replicas unavailable
                         -> Try: sikifanso app disable qdrant
```

Each failure includes the root cause and a suggested fix command.

If no cluster session exists, `doctor` still runs the Docker check and reports the missing cluster.

## Snapshots

```bash
sikifanso snapshot capture --name before-upgrade
sikifanso snapshot list
sikifanso snapshot restore before-upgrade
```

Capture and restore cluster configuration state (session metadata + gitops repo). Snapshots are stored at `~/.sikifanso/snapshots/`.

## Dashboard

```bash
sikifanso cluster dashboard
```

Starts a local web dashboard at `http://localhost:9090`. Opens your browser automatically.

## Multi-cluster

Each cluster gets its own ports, kubeconfig context, gitops repo, and session:

```bash
sikifanso cluster create --name lab1 --profile agent-dev
sikifanso cluster create --name lab2 --profile agent-minimal

# Deploy to a specific cluster
sikifanso app sync --cluster lab1

# See all clusters
sikifanso cluster info
```

Ports are auto-resolved -- if defaults (30080, 30081, etc.) are taken by the first cluster, the next one gets free ports automatically.

## Architecture

```
~/.sikifanso/clusters/<name>/
+-- session.yaml              # Cluster metadata, credentials, ports
+-- gitops/                   # Local git repo (mounted into cluster)
    +-- bootstrap/
    |   +-- root-app.yaml     # ApplicationSet for custom apps
    |   +-- root-catalog.yaml # ApplicationSet for catalog apps
    +-- apps/                 # Custom user-supplied Helm apps
    |   +-- coordinates/
    |   |   +-- <app>.yaml    # Helm chart coordinates (repo, chart, version, namespace)
    |   +-- values/
    |       +-- <app>.yaml    # Helm values overrides
    +-- catalog/              # AI agent infrastructure catalog
    |   +-- <app>.yaml        # App definition with enabled flag
    |   +-- values/
    |       +-- <app>.yaml    # Helm values overrides
    +-- agents/               # Agent sandbox definitions
        +-- <agent>.yaml
        +-- values/
            +-- <agent>.yaml
```

The gitops directory is mounted into the k3d cluster at `/local-gitops` via a hostPath volume. ArgoCD's repo-server reads from it directly -- no remote git server needed.

Two root ApplicationSets manage the dual-track app model:

- **root-app.yaml** watches `apps/coordinates/*.yaml` for custom Helm charts added via `app add`
- **root-catalog.yaml** watches `catalog/*.yaml` and deploys only entries where `enabled: true`

### How `app sync` works

ArgoCD's default reconciliation interval is 180 seconds. The sync command bypasses this by sending a webhook push event (mimicking a GitHub push notification) to two endpoints:

1. **ArgoCD server** -- invalidates the repo-server's git revision cache
2. **ApplicationSet controller** -- triggers immediate re-evaluation of the git generator

The ApplicationSet controller webhook is reached via the Kubernetes API server proxy (no extra ports exposed).

## State management

Cluster metadata is persisted to `~/.sikifanso/clusters/<name>/session.yaml` and includes:

- Cluster state (running / stopped)
- ArgoCD URL, username, password
- Hubble UI URL
- GitOps repo path
- k3d configuration (image, node counts)
- Bootstrap template URL and version
