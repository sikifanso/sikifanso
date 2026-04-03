# CLI Reference

## Usage

```
sikifanso [global flags] <command> [command flags]
```

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--cluster`, `-c` | `default` | Target cluster name |
| `--output`, `-o` | `table` | Output format (`table`, `json`) |
| `--log-level` | `info` | Console log level (`debug`, `info`, `warn`, `error`) |

The `--cluster` flag can also be set via the `SIKIFANSO_CLUSTER` environment variable.

---

## `cluster` -- Manage local Kubernetes clusters

### `cluster create`

Create a new k3d cluster with Cilium and ArgoCD pre-configured.

```bash
sikifanso cluster create
sikifanso cluster create --name mylab
sikifanso cluster create --profile agent-dev
sikifanso cluster create --name mylab --profile agent-dev,rag
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `default` | Cluster name |
| `--bootstrap` | *(sikifanso default)* | Bootstrap template repo URL |
| `--bootstrap-version` | *(match CLI version)* | Bootstrap repo tag to clone (empty string forces HEAD) |
| `--profile` | *(none)* | Enable a predefined set of catalog apps (comma-separated for composition) |

If flags are omitted, the CLI prompts interactively. For release builds using the default bootstrap repo, the CLI automatically pins to the matching bootstrap tag. Dev builds and custom bootstrap repos default to HEAD.

See [Profiles](guides/profiles.md) for available profiles and composition.

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

Show cluster details, credentials, and runtime health. Omit the name to list all clusters.

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

### `cluster doctor`

Run health checks on the cluster and its components. Exits 0 when all checks pass, 1 when any check fails.

```bash
sikifanso cluster doctor
sikifanso cluster doctor --cluster mylab
```

Checks run in order:

| Check | What it verifies |
|-------|-----------------|
| Docker daemon | Docker is reachable; reports version |
| k3d cluster | All k3d nodes are in Ready state |
| Cilium | `cilium` DaemonSet in `kube-system` is fully available |
| Hubble | `hubble-relay` Deployment in `kube-system` is Available |
| ArgoCD | Core deployments (`argocd-server`, `argocd-repo-server`, `argocd-applicationset-controller`) are Available |
| Catalog apps | Each enabled catalog app's ArgoCD Application is Healthy and Synced |
| Agents | Each agent namespace is properly deployed |

Each failure includes a cause and a suggested fix command:

```
ok  Docker daemon       running (v27.0.3)
ok  k3d cluster         1/1 nodes ready
ok  Cilium              DaemonSet 1/1 ready
ok  Hubble              relay deployment ready
ok  ArgoCD              3/3 deployments ready
!!  App: grafana         Degraded -- Synced
                         -> Deployment grafana in namespace monitoring: replicas unavailable
                         -> Try: sikifanso app disable grafana
```

If no cluster session exists, `doctor` runs the Docker check only and reports the missing cluster with a suggested `sikifanso cluster create` fix.

### `cluster dashboard`

Start the local web dashboard. Opens a browser automatically unless `--no-browser` is set. Press Ctrl+C to stop.

```bash
sikifanso cluster dashboard
sikifanso cluster dashboard --addr :8080
sikifanso cluster dashboard --no-browser
```

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `:9090` | Listen address |
| `--no-browser` | `false` | Don't open browser automatically |

### `cluster upgrade`

Upgrade cluster components (Cilium and ArgoCD). Takes a pre-upgrade snapshot by default. Without `--all`, shows subcommand help.

```bash
sikifanso cluster upgrade --all
sikifanso cluster upgrade --all --skip-snapshot
sikifanso cluster upgrade cilium
sikifanso cluster upgrade argocd
```

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Upgrade all components |
| `--skip-snapshot` | `false` | Skip pre-upgrade snapshot |

#### `cluster upgrade cilium`

Upgrade Cilium CNI only.

| Flag | Default | Description |
|------|---------|-------------|
| `--skip-snapshot` | `false` | Skip pre-upgrade snapshot |

#### `cluster upgrade argocd`

Upgrade ArgoCD only.

| Flag | Default | Description |
|------|---------|-------------|
| `--skip-snapshot` | `false` | Skip pre-upgrade snapshot |

### `cluster profiles`

List available cluster profiles for the `--profile` flag. Shows each profile's name, description, and included apps.

```bash
sikifanso cluster profiles
```

---

## `app` -- Manage applications

Unified management for both catalog apps and custom Helm charts.

### `app add [NAME]`

Add a custom Helm chart to the gitops repo. Writes a coordinate file and a stub values file, auto-commits, and triggers an ArgoCD sync. For curated apps, use `app enable` instead.

```bash
sikifanso app add podinfo --repo https://stefanprodan.github.io/podinfo --chart podinfo --version 6.10.1 --namespace podinfo
sikifanso app add   # interactive — see below
```

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | *(prompted)* | Helm repository URL |
| `--chart` | *(app name)* | Chart name within the repository |
| `--version` | `*` | Chart version (targetRevision) |
| `--namespace` | *(app name)* | Kubernetes namespace to deploy into |
| `--no-wait` | `false` | Trigger sync without waiting |
| `--timeout` | `2m` | Timeout for sync wait |

**Interactive mode** has two paths:

- **No args, no flags, TTY** -- launches a TUI catalog browser where you can toggle catalog apps on/off with a single keypress
- **Name given but flags missing** -- prompts for each missing field individually

Creates two files in the gitops repo:

- `apps/coordinates/<name>.yaml` -- Helm chart coordinates
- `apps/values/<name>.yaml` -- stub values file

### `app list`

List all installed apps in the current cluster's gitops repo. Shows both custom apps and enabled catalog apps, with a `SOURCE` column to distinguish them.

```bash
sikifanso app list
sikifanso app list --all
```

| Flag | Default | Description |
|------|---------|-------------|
| `--all`, `-a` | `false` | Show all catalog entries including disabled |

```
NAME                 CHART                  VERSION    NAMESPACE    SOURCE
litellm-proxy        litellm                0.2.1      gateway      catalog
langfuse             langfuse               1.2.14     observability catalog
podinfo              podinfo                6.10.1     podinfo      custom
```

### `app remove NAME`

Remove a custom app from the gitops repo. Deletes the coordinate and values files, auto-commits, and triggers an ArgoCD sync. To disable a catalog app, use `app disable` instead.

```bash
sikifanso app remove podinfo
```

| Argument | Description |
|----------|-------------|
| `NAME` | App name to remove (required) |

| Flag | Default | Description |
|------|---------|-------------|
| `--no-wait` | `false` | Trigger sync without waiting |
| `--timeout` | `2m` | Timeout for sync wait |

Shell completion is supported -- press Tab to see available app names.

### `app enable NAME`

Enable a catalog application. Sets `enabled: true` in the catalog entry, commits, and triggers an ArgoCD sync.

```bash
sikifanso app enable litellm-proxy
```

| Argument | Description |
|----------|-------------|
| `NAME` | Catalog app name to enable (required) |

| Flag | Default | Description |
|------|---------|-------------|
| `--no-wait` | `false` | Trigger sync without waiting |
| `--timeout` | `2m` | Timeout for sync wait |

If the app is already enabled, prints a message and does nothing. Shell completion suggests disabled catalog app names.

### `app disable NAME`

Disable a catalog application. Sets `enabled: false` in the catalog entry, commits, and triggers an ArgoCD sync.

```bash
sikifanso app disable litellm-proxy
```

| Argument | Description |
|----------|-------------|
| `NAME` | Catalog app name to disable (required) |

| Flag | Default | Description |
|------|---------|-------------|
| `--no-wait` | `false` | Trigger sync without waiting |
| `--timeout` | `2m` | Timeout for sync wait |

If the app is already disabled, prints a message and does nothing. Shell completion suggests enabled catalog app names.

### `app sync`

Trigger ArgoCD sync for all or specific applications. Bypasses the default 3-minute polling interval.

```bash
sikifanso app sync
sikifanso app sync --app podinfo
sikifanso app sync --no-wait
```

| Flag | Default | Description |
|------|---------|-------------|
| `--no-wait` | `false` | Trigger sync without waiting for completion |
| `--app` | *(none)* | Sync a specific application by name |
| `--timeout` | `2m` | Timeout for sync wait |
| `--skip-unhealthy` | `false` | Ignore pre-existing Degraded apps |

When `--app` is set, only that application is synced. By default, the command waits for all applications to reach Synced/Healthy or timeout.

### `app status [APP]`

Show detailed application status with resource tree. Omit the app name to show all apps.

```bash
sikifanso app status
sikifanso app status litellm-proxy
```

| Argument | Default | Description |
|----------|---------|-------------|
| `APP` | *(all)* | Application name to inspect |

### `app diff APP`

Show diff between live and desired state for an application.

```bash
sikifanso app diff litellm-proxy
```

| Argument | Description |
|----------|-------------|
| `APP` | Application name (required) |

### `app logs APP`

Stream pod logs for an application.

```bash
sikifanso app logs litellm-proxy --pod litellm-proxy-0
sikifanso app logs litellm-proxy --pod litellm-proxy-0 --follow
```

| Argument | Description |
|----------|-------------|
| `APP` | Application name (required) |

| Flag | Default | Description |
|------|---------|-------------|
| `--pod` | *(required)* | Pod name to fetch logs from |
| `--container` | *(first)* | Container name (optional) |
| `--follow`, `-f` | `false` | Stream logs continuously |

### `app rollback APP`

Roll back an application to a previous revision.

```bash
sikifanso app rollback litellm-proxy
sikifanso app rollback litellm-proxy --revision 3
```

| Argument | Description |
|----------|-------------|
| `APP` | Application name (required) |

| Flag | Default | Description |
|------|---------|-------------|
| `--revision` | `0` | History revision ID to rollback to (0 = previous) |

---

## `agent` -- Manage isolated agent namespaces

See [Agent Sandboxes](guides/agent-sandboxes.md) for a full guide.

### `agent create NAME`

Create an isolated agent namespace with resource quotas and network policies.

```bash
sikifanso agent create my-agent
sikifanso agent create my-agent --cpu 1 --memory 1Gi --pods 20
```

| Argument | Description |
|----------|-------------|
| `NAME` | Agent name (required) |

| Flag | Default | Description |
|------|---------|-------------|
| `--cpu` | `500m` | CPU quota |
| `--memory` | `512Mi` | Memory quota |
| `--pods` | `10` | Max pods |
| `--no-wait` | `false` | Trigger sync without waiting |
| `--timeout` | `2m` | Timeout for sync wait |

### `agent list`

List all agent namespaces with their resource quotas.

```bash
sikifanso agent list
```

### `agent delete NAME`

Delete an agent namespace and clean up all resources.

```bash
sikifanso agent delete my-agent
```

| Argument | Description |
|----------|-------------|
| `NAME` | Agent name to delete (required) |

| Flag | Default | Description |
|------|---------|-------------|
| `--no-wait` | `false` | Trigger sync without waiting |
| `--timeout` | `2m` | Timeout for sync wait |

---

## `snapshot` -- Capture, restore, and manage cluster snapshots

### `snapshot capture`

Capture the cluster's configuration state (session metadata + gitops repo) into a `.tar.gz` archive stored at `~/.sikifanso/snapshots/`.

```bash
sikifanso snapshot capture --name before-upgrade
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | *(required)* | Snapshot name |

### `snapshot list`

List all available snapshots. Shows name, cluster, creation time, and CLI version.

```bash
sikifanso snapshot list
```

### `snapshot restore NAME`

Restore a cluster's configuration from a snapshot. This restores the session metadata and gitops repo only -- you must run `sikifanso cluster create` afterward to recreate the cluster infrastructure.

```bash
sikifanso snapshot restore before-upgrade
```

| Argument | Description |
|----------|-------------|
| `NAME` | Snapshot name to restore (required) |

Shell completion is supported.

### `snapshot delete NAME`

Delete a snapshot archive.

```bash
sikifanso snapshot delete old-snapshot
```

| Argument | Description |
|----------|-------------|
| `NAME` | Snapshot name to delete (required) |

Shell completion is supported.

---

## `mcp serve`

Start the MCP (Model Context Protocol) server on stdio transport. See [MCP Server](guides/mcp-server.md) for setup and tool catalog.

```bash
sikifanso mcp serve
```

---

## Environment variables

| Variable | Description |
|----------|-------------|
| `SIKIFANSO_CLUSTER` | Default cluster name (same as `--cluster` flag) |
