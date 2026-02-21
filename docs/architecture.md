# Architecture

This page explains how sikifanso sets up and manages your homelab cluster.

## Directory structure

Each cluster's state lives under `~/.sikifanso/clusters/<name>/`:

```
~/.sikifanso/clusters/<name>/
├── session.yaml              # Cluster metadata, credentials, ports
└── gitops/                   # Local git repo (mounted into cluster)
    ├── bootstrap/
    │   └── root-app.yaml     # Root ApplicationSet manifest
    └── apps/
        └── <app>/
            └── config.yaml   # Helm chart definition
```

## How the local gitops repo works

The `gitops/` directory is a regular git repository on your filesystem. During cluster creation, it is scaffolded from a bootstrap template repo.

This directory is mounted into the k3d cluster at `/local-gitops` via a **hostPath volume**. ArgoCD's repo-server reads from it directly — no remote git server needed.

## Root ApplicationSet

The bootstrap template includes a root `ApplicationSet` that uses the **git file generator**. It watches `apps/*/config.yaml` in the gitops repo.

Each `config.yaml` defines a Helm chart source:

```yaml
name: podinfo
repoURL: https://stefanprodan.github.io/podinfo
chart: podinfo
targetRevision: 6.10.1
namespace: podinfo
```

The ApplicationSet automatically creates an ArgoCD `Application` for every matching config file. Adding or removing an app directory is all it takes to deploy or undeploy.

## How `argocd sync` works

ArgoCD's default reconciliation interval is **180 seconds**. The `sikifanso argocd sync` command bypasses this by sending a webhook push event (mimicking a GitHub push notification) to two endpoints:

1. **ArgoCD server** — invalidates the repo-server's git revision cache, causing it to re-read the local gitops repo
2. **ApplicationSet controller** — triggers immediate re-evaluation of the git generator, picking up new or removed app directories

The ApplicationSet controller webhook is reached via the **Kubernetes API server proxy**, so no extra ports are exposed.

## Cluster components

When you run `sikifanso cluster create`, the following happens in order:

1. **k3d cluster** is created with 1 server + 2 agents running k3s v1.29. The flannel CNI and kube-proxy are disabled (Cilium replaces both).
2. **Cilium** is installed via Helm as a full kube-proxy replacement with ingress controller and Hubble UI enabled.
3. **ArgoCD** is installed via Helm, configured to use the hostPath-mounted gitops repo as its source.
4. **GitOps repo** is cloned from the bootstrap template and the root ApplicationSet is applied.
5. **Ports** are mapped from the cluster to your host (ArgoCD UI, Hubble UI).

## State management

Cluster metadata is persisted to `~/.sikifanso/clusters/<name>/session.yaml` and includes:

- Cluster state (running / stopped)
- ArgoCD URL, username, password
- Hubble UI URL
- GitOps repo path
- k3d configuration (image, node counts)
- Port mappings
- Bootstrap template URL

This file is read on every CLI command to locate and interact with the cluster.

## Port allocation

Each cluster gets its own set of ports. Defaults are:

- **30080** — ArgoCD UI
- **30081** — Hubble UI

If defaults are taken by another cluster, sikifanso automatically finds free ports. Port assignments are stored in `session.yaml`.
