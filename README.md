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


A CLI tool that bootstraps a fully functional homelab Kubernetes cluster with a single command. Spins up a [k3d](https://k3d.io) cluster pre-configured with [Cilium](https://cilium.io) (eBPF networking, ingress, Hubble observability) and [ArgoCD](https://argoproj.github.io/cd/) (GitOps from a local git repository).

## What you get

```
sikifanso cluster create
```

- **k3d cluster** — 1 server + 2 agents, k3s v1.29
- **Cilium** — full kube-proxy replacement, ingress controller, Hubble UI
- **ArgoCD** — configured to read from a local gitops repo on your filesystem
- **GitOps repo** — scaffolded from a bootstrap template, mounted into the cluster
- **Root ApplicationSet** — watches `apps/coordinates/*.yaml` in your gitops repo and deploys them automatically

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
# Create a cluster (interactive prompts for name and bootstrap repo)
sikifanso cluster create

# Or with explicit flags
sikifanso cluster create --name mylab --bootstrap https://github.com/sikifanso/sikifanso-homelab-bootstrap.git
```

After creation you'll see:

```
╭──────────────────────────────────────────╮
│           Cluster: default               │
├──────────────────────────────────────────┤
│ State:           running                 │
│                                          │
│ ArgoCD URL:      http://localhost:30080  │
│ ArgoCD User:     admin                   │
│ ArgoCD Password: ••••••••                │
│                                          │
│ Hubble URL:      http://localhost:30081  │
│                                          │
│ GitOps Path:     ~/.sikifanso/clusters/… │
╰──────────────────────────────────────────╯
```

## Deploying apps

Use the `app add` command to deploy a Helm chart:

```bash
sikifanso app add podinfo \
  --repo https://stefanprodan.github.io/podinfo \
  --chart podinfo \
  --version 6.10.1 \
  --namespace podinfo
```

This writes the chart coordinates and a stub values file to your gitops repo, auto-commits, and triggers an ArgoCD sync. If you omit any flags, the CLI prompts interactively.

```bash
# List installed apps
sikifanso app list
```

```
NAME                 CHART           VERSION    NAMESPACE
podinfo              podinfo         6.10.1     podinfo
```

Remove an app with `app remove`:

```bash
sikifanso app remove podinfo
```

You can also create the files manually under `apps/coordinates/` and `apps/values/` if you prefer — see [Architecture](docs/architecture.md) for the file format.

## CLI reference

```
sikifanso [global flags] <command> [command flags]
```

### Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--cluster`, `-c` | `default` | Target cluster name |
| `--log-file` | `sikifanso.log` | Path to log file |
| `--log-level` | `info` | Console log level (debug, info, warn, error) |

The `--cluster` flag can also be set via `SIKIFANSO_CLUSTER` env var.

### Commands

| Command | Description |
|---------|-------------|
| `cluster create` | Create a new cluster |
| `cluster delete [NAME]` | Delete a cluster and clean up |
| `cluster info [NAME]` | Show cluster details (omit name to list all) |
| `cluster start [NAME]` | Start a stopped cluster |
| `cluster stop [NAME]` | Stop a running cluster |
| `app add [NAME]` | Add a Helm chart to the gitops repo |
| `app list` | List installed apps |
| `app remove NAME` | Remove an app from the gitops repo |
| `status [NAME]` | Show cluster state, nodes, and pods |
| `argocd sync` | Force immediate ArgoCD reconciliation |

## Multi-cluster

Each cluster gets its own ports, kubeconfig context, gitops repo, and session:

```bash
sikifanso cluster create --name lab1
sikifanso cluster create --name lab2

# Deploy to a specific cluster
sikifanso argocd sync --cluster lab1
sikifanso argocd sync --cluster lab2

# See all clusters
sikifanso cluster info
```

Ports are auto-resolved — if defaults (30080, 30081, etc.) are taken by the first cluster, the next one gets free ports automatically.

## Architecture

```
~/.sikifanso/clusters/<name>/
├── session.yaml              # Cluster metadata, credentials, ports
└── gitops/                   # Local git repo (mounted into cluster)
    ├── bootstrap/
    │   └── root-app.yaml     # Root ApplicationSet manifest
    └── apps/
        ├── coordinates/
        │   └── <app>.yaml    # Helm chart coordinates (repo, chart, version, namespace)
        └── values/
            └── <app>.yaml    # Helm values overrides
```

The gitops directory is mounted into the k3d cluster at `/local-gitops` via a hostPath volume. ArgoCD's repo-server reads from it directly — no remote git server needed.

The root ApplicationSet watches `apps/coordinates/*.yaml` with a git file generator. Each coordinate file defines a Helm chart source, and the ApplicationSet creates a multi-source ArgoCD Application that pairs it with the matching values file.

### How `argocd sync` works

ArgoCD's default reconciliation interval is 180 seconds. The sync command bypasses this by sending a webhook push event (mimicking a GitHub push notification) to two endpoints:

1. **ArgoCD server** — invalidates the repo-server's git revision cache
2. **ApplicationSet controller** — triggers immediate re-evaluation of the git generator

The ApplicationSet controller webhook is reached via the Kubernetes API server proxy (no extra ports exposed).

## State management

Cluster metadata is persisted to `~/.sikifanso/clusters/<name>/session.yaml` and includes:

- Cluster state (running / stopped)
- ArgoCD URL, username, password
- Hubble UI URL
- GitOps repo path
- k3d configuration (image, node counts)
- Bootstrap template URL