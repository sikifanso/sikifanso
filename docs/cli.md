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
| `--bootstrap-version` | *(match CLI version)* | Bootstrap repo tag to clone (empty string forces HEAD) |

If flags are omitted, the CLI prompts interactively. For release builds using the default bootstrap repo, the CLI automatically pins to the matching bootstrap tag. Dev builds and custom bootstrap repos default to HEAD.

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

Add a custom Helm chart to the gitops repo. Writes a coordinate file and a stub values file, auto-commits, and triggers an ArgoCD sync. For curated apps, use `catalog enable` instead.

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

List all installed apps in the current cluster's gitops repo. Shows both custom apps (from `apps/coordinates/`) and enabled catalog apps, with a `SOURCE` column to distinguish them.

```bash
sikifanso app list
```

```
NAME                 CHART                  VERSION    NAMESPACE    SOURCE
podinfo              podinfo                6.10.1     podinfo      custom
prometheus-stack     kube-prometheus-stack   82.4.3   monitoring   catalog
```

No flags beyond the global `--cluster`.

### `app remove NAME`

Remove a custom app from the gitops repo. Deletes the coordinate and values files, auto-commits, and triggers an ArgoCD sync. To disable a catalog app, use `catalog disable` instead.

```bash
sikifanso app remove podinfo
```

| Argument | Description |
|----------|-------------|
| `NAME` | App name to remove (required) |

Shell completion is supported -- press Tab to see available app names.

### `catalog list`

List all catalog apps with their enabled/disabled status.

```bash
sikifanso catalog list
```

```
NAME                CATEGORY     ENABLED  DESCRIPTION
alertmanager        monitoring   false    Alertmanager for Prometheus alerts
grafana             monitoring   false    Grafana observability dashboards
prometheus-stack    monitoring   true     Prometheus metrics collection and Grafana dashboards
```

Columns are: `NAME`, `CATEGORY`, `ENABLED`, `DESCRIPTION`. The `ENABLED` column is color-coded green/red.

### `catalog enable NAME`

Enable a catalog app. Sets `enabled: true` in the catalog entry, commits, and triggers an ArgoCD sync.

```bash
sikifanso catalog enable prometheus-stack
```

| Argument | Description |
|----------|-------------|
| `NAME` | Catalog app name to enable (required) |

If the app is already enabled, prints a message and does nothing. If the app name is not found, returns an error listing all available catalog apps.

Shell completion is supported -- press Tab to see available catalog app names.

### `catalog disable NAME`

Disable a catalog app. Sets `enabled: false` in the catalog entry, commits, and triggers an ArgoCD sync.

```bash
sikifanso catalog disable prometheus-stack
```

| Argument | Description |
|----------|-------------|
| `NAME` | Catalog app name to disable (required) |

If the app is already disabled, prints a message and does nothing.

Shell completion is supported -- press Tab to see currently enabled catalog app names.

### `doctor`

Run health checks on the cluster and its components. Exits 0 when all checks pass, 1 when any check fails.

```bash
sikifanso doctor
sikifanso doctor --cluster mylab
```

Checks run in order:

| Check | What it verifies |
|-------|-----------------|
| Docker daemon | Docker is reachable; reports version |
| k3d cluster | All k3d nodes are in Ready state |
| Cilium | `cilium` DaemonSet in `kube-system` is fully available |
| Hubble | `hubble-relay` Deployment in `kube-system` is Available |
| ArgoCD | Core deployments (`argocd-server`, `argocd-repo-server`, `argocd-applicationset-controller`) are Available |
| Apps | Each enabled catalog app's ArgoCD Application is Healthy and Synced |

Each failure includes a cause and a suggested fix command:

```
ok  Docker daemon       running (v27.0.3)
ok  k3d cluster         3/3 nodes ready
ok  Cilium              DaemonSet 3/3 ready
ok  Hubble              relay deployment ready
ok  ArgoCD              3/3 deployments ready
!!  App: grafana         Degraded -- Synced
                         -> Deployment grafana in namespace monitoring: replicas unavailable
                         -> Try: sikifanso catalog disable grafana
```

If no cluster session exists (no cluster has been created), `doctor` runs the Docker check only and reports the missing cluster with a suggested `sikifanso cluster create` fix.

No flags beyond the global `--cluster`.

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
