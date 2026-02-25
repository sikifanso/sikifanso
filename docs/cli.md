# CLI Reference

## Usage

```
sikifanso [global flags] <command> [command flags]
```

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--cluster`, `-c` | `default` | Target cluster name |
| `--log-file` | `sikifanso.log` | Path to log file |
| `--log-level` | `info` | Console log level (`debug`, `info`, `warn`, `error`) |

The `--cluster` flag can also be set via the `SIKIFANSO_CLUSTER` environment variable.

## Commands

### `cluster create`

Create a new k3d cluster with Cilium and ArgoCD pre-configured.

```bash
sikifanso cluster create
sikifanso cluster create --name mylab
sikifanso cluster create --name mylab --bootstrap https://github.com/sikifanso/sikifanso-homelab-bootstrap.git
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `default` | Cluster name |
| `--bootstrap` | sikifanso default | Bootstrap template repo URL |

If flags are omitted, the CLI prompts interactively.

### `cluster delete [NAME]`

Delete a cluster and clean up all resources.

```bash
sikifanso cluster delete
sikifanso cluster delete mylab
```

| Argument | Default | Description |
|----------|---------|-------------|
| `NAME` | `default` | Cluster name to delete |

### `cluster info [NAME]`

Show cluster details. Omit the name to list all clusters.

```bash
sikifanso cluster info
sikifanso cluster info mylab
```

| Argument | Default | Description |
|----------|---------|-------------|
| `NAME` | *(all)* | Cluster name to inspect |

### `cluster start [NAME]`

Start a previously stopped cluster.

```bash
sikifanso cluster start
sikifanso cluster start mylab
```

| Argument | Default | Description |
|----------|---------|-------------|
| `NAME` | `default` | Cluster name to start |

### `cluster stop [NAME]`

Stop a running cluster without deleting it. State is preserved.

```bash
sikifanso cluster stop
sikifanso cluster stop mylab
```

| Argument | Default | Description |
|----------|---------|-------------|
| `NAME` | `default` | Cluster name to stop |

### `app add [NAME]`

Add a Helm chart to the gitops repo. Writes a coordinate file and a stub values file, auto-commits, and triggers an ArgoCD sync.

```bash
sikifanso app add podinfo --repo https://stefanprodan.github.io/podinfo --chart podinfo --version 6.10.1 --namespace podinfo
sikifanso app add   # interactive — prompts for all fields
```

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | *(prompted)* | Helm repository URL |
| `--chart` | *(app name)* | Chart name within the repository |
| `--version` | `*` | Chart version (targetRevision) |
| `--namespace` | *(app name)* | Kubernetes namespace to deploy into |

If any flag is omitted, the CLI prompts interactively. The name can be passed as a positional argument or entered at the prompt.

Creates two files in the gitops repo:

- `apps/coordinates/<name>.yaml` — Helm chart coordinates
- `apps/values/<name>.yaml` — stub values file (`# Helm values for <name>`)

### `app list`

List all installed apps in the current cluster's gitops repo.

```bash
sikifanso app list
```

```
NAME                 CHART           VERSION    NAMESPACE
podinfo              podinfo         6.10.1     podinfo
```

No flags beyond the global `--cluster`.

### `app remove NAME`

Remove an app from the gitops repo. Deletes the coordinate and values files, auto-commits, and triggers an ArgoCD sync.

```bash
sikifanso app remove podinfo
```

| Argument | Description |
|----------|-------------|
| `NAME` | App name to remove (required) |

Shell completion is supported — press Tab to see available app names.

### `status [NAME]`

Show cluster state, nodes, and pod summary. Omit the name to show all clusters.

```bash
sikifanso status
sikifanso status mylab
```

| Argument | Default | Description |
|----------|---------|-------------|
| `NAME` | *(all)* | Cluster name to inspect |

Displays session metadata (state, creation time, node count), a node readiness table, and a per-namespace pod summary. If the cluster is not running, Kubernetes queries are skipped.

### `argocd sync`

Force immediate ArgoCD reconciliation. Bypasses the default 3-minute polling interval.

```bash
sikifanso argocd sync
sikifanso argocd sync --cluster mylab
```

Uses the `--cluster` global flag to target a specific cluster.

## Environment variables

| Variable | Description |
|----------|-------------|
| `SIKIFANSO_CLUSTER` | Default cluster name (same as `--cluster` flag) |
