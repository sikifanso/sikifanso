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
