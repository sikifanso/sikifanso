# Architecture

This page explains how sikifanso sets up and manages Kubernetes clusters for AI agent infrastructure.

## Directory structure

Each cluster's state lives under `~/.sikifanso/clusters/<name>/`:

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
        +-- <app>.yaml        # App definition with enabled flag
        +-- values/
            +-- <app>.yaml    # Helm values overrides
```

## How the local gitops repo works

The `gitops/` directory is a regular git repository on your filesystem. During cluster creation, it is scaffolded from a bootstrap template repo.

This directory is mounted into the k3d cluster at `/local-gitops` via a **hostPath volume**. ArgoCD's repo-server reads from it directly -- no remote git server needed.

## AI Agent Infrastructure Catalog

The catalog contains curated tools organized by function:

| Category | Tools | Purpose |
|----------|-------|---------|
| **gateway** | LiteLLM Proxy | LLM API routing, cost tracking, multi-provider support |
| **observability** | Langfuse, Prometheus+Grafana, Loki, Tempo | LLM tracing, metrics, logs, distributed tracing |
| **guardrails** | Guardrails AI, NeMo Guardrails, Presidio | Output validation, safety rails, PII redaction |
| **rag** | Qdrant, Text Embeddings Inference, Unstructured | Vector storage, embeddings, document parsing |
| **runtime** | Temporal, External Secrets, OPA | Workflow orchestration, secrets management, policy |
| **models** | Ollama | Local LLM inference and model management |
| **storage** | PostgreSQL, Valkey (Redis) | Supporting data stores for other tools |

Each tool is a self-contained YAML file with Helm chart coordinates and an `enabled` flag. ArgoCD's ApplicationSet deploys only enabled entries.

## Root ApplicationSets

The bootstrap template includes two `ApplicationSet` manifests that manage two tracks of apps:

### Custom apps (`root-app.yaml`)

Uses the **git file generator** to watch `apps/coordinates/*.yaml`. Each coordinate file defines a Helm chart source:

```yaml
name: podinfo
repoURL: https://stefanprodan.github.io/podinfo
chart: podinfo
targetRevision: 6.10.1
namespace: podinfo
```

The ApplicationSet creates a multi-source ArgoCD `Application` for every matching coordinate file, pairing it with the corresponding values file at `apps/values/<name>.yaml`. Adding or removing a coordinate file is all it takes to deploy or undeploy.

### Catalog apps (`root-catalog.yaml`)

Uses the **git file generator** to watch `catalog/*.yaml`, but only generates Applications for entries where `enabled: true`. Each catalog entry has additional metadata:

```yaml
name: litellm-proxy
category: gateway
description: LLM API gateway with multi-provider routing, cost tracking, and rate limiting
repoURL: https://litellm.github.io/helm-charts
chart: litellm
targetRevision: "0.2.1"
namespace: gateway
enabled: false
```

Setting `enabled: true` and committing causes ArgoCD to create and sync the Application. Setting it back to `false` causes ArgoCD to prune/delete it. Values overrides live at `catalog/values/<name>.yaml`.

## How `argocd sync` works

ArgoCD's default reconciliation interval is **180 seconds**. The `sikifanso argocd sync` command bypasses this by sending a webhook push event (mimicking a GitHub push notification) to two endpoints:

1. **ArgoCD server** -- invalidates the repo-server's git revision cache, causing it to re-read the local gitops repo
2. **ApplicationSet controller** -- triggers immediate re-evaluation of the git generator, picking up new or removed coordinate files

The ApplicationSet controller webhook is reached via the **Kubernetes API server proxy**, so no extra ports are exposed.

## Cluster components

When you run `sikifanso cluster create`, the following happens in order:

1. **k3d cluster** is created with 1 server + 2 agents running k3s v1.29. The flannel CNI and kube-proxy are disabled (Cilium replaces both).
2. **Cilium** is installed via Helm as a full kube-proxy replacement with ingress controller and Hubble UI enabled.
3. **ArgoCD** is installed via Helm, configured to use the hostPath-mounted gitops repo as its source.
4. **GitOps repo** is cloned from the bootstrap template and both root ApplicationSets are applied.
5. **Ports** are mapped from the cluster to your host (ArgoCD UI, Hubble UI).

## State management

Cluster metadata is persisted to `~/.sikifanso/clusters/<name>/session.yaml` and includes:

- Cluster state (running / stopped)
- ArgoCD URL, username, password
- Hubble UI URL
- GitOps repo path
- k3d configuration (image, node counts)
- Port mappings
- Bootstrap template URL and version

This file is read on every CLI command to locate and interact with the cluster.

## Snapshot storage

Snapshots are stored at `~/.sikifanso/snapshots/<name>.tar.gz`. Each archive contains the session metadata and the full gitops repo directory, allowing a cluster's configuration to be captured and restored independently of the running infrastructure.

## Port allocation

Each cluster gets its own set of ports. Defaults are:

- **30080** -- ArgoCD UI
- **30081** -- Hubble UI

If defaults are taken by another cluster, sikifanso automatically finds free ports. Port assignments are stored in `session.yaml`.

The dashboard listens on `:9090` by default, independent of cluster port mappings (configurable via `--addr`).
