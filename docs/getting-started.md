# Getting Started

This guide walks you through creating your first cluster, deploying an app, and syncing changes.

## Create a cluster

```bash
sikifanso cluster create
```

The CLI will prompt you for a cluster name and bootstrap repo URL. You can also pass them directly:

```bash
sikifanso cluster create --name mylab --bootstrap https://github.com/sikifanso/sikifanso-homelab-bootstrap.git
```

This takes a few minutes. When it's done, you'll see:

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

Open the **ArgoCD URL** in your browser and log in with the displayed credentials. You'll see ArgoCD and Cilium already deployed and healthy.

Open the **Hubble URL** to see real-time network traffic in your cluster.

## Deploy an app from the catalog

The bootstrap repo ships with a curated catalog of apps. Enable one with:

```bash
sikifanso catalog enable prometheus-stack
```

This sets `enabled: true` in the catalog entry, commits the change, and triggers an ArgoCD sync.

Browse all available catalog apps with:

```bash
sikifanso catalog list
```

## Deploy a custom Helm chart

You can also deploy any Helm chart directly. Let's deploy [podinfo](https://github.com/stefanprodan/podinfo) as an example.

```bash
sikifanso app add podinfo \
  --repo https://stefanprodan.github.io/podinfo \
  --chart podinfo \
  --version 6.10.1 \
  --namespace podinfo
```

This writes the chart coordinates and a stub values file to your gitops repo, auto-commits, and triggers an ArgoCD sync. If you omit any flags, the CLI prompts interactively.

## List installed apps

```bash
sikifanso app list
```

```
NAME                 CHART                  VERSION    NAMESPACE    SOURCE
podinfo              podinfo                6.10.1     podinfo      custom
prometheus-stack     kube-prometheus-stack   82.4.3   monitoring   catalog
```

Both custom apps and enabled catalog apps are shown, with a `SOURCE` column to distinguish them.

You can also create the files manually under `apps/coordinates/` and `apps/values/` in your gitops repo -- see [Architecture](architecture.md) for the file format.

## Remove an app

For custom apps:

```bash
sikifanso app remove podinfo
```

This deletes the coordinate and values files, auto-commits, and triggers an ArgoCD sync.

For catalog apps:

```bash
sikifanso catalog disable prometheus-stack
```

This sets `enabled: false`, commits, and triggers an ArgoCD sync.

## Check cluster health

Run `doctor` to verify that all cluster components are healthy:

```bash
sikifanso doctor
```

This checks Docker, k3d nodes, Cilium, Hubble, ArgoCD, and every enabled catalog app. If anything is wrong, it tells you exactly what failed and how to fix it.

## Manage your cluster

```bash
# Show cluster details
sikifanso cluster info

# Check cluster health
sikifanso doctor

# Stop the cluster (preserves state)
sikifanso cluster stop

# Start it again
sikifanso cluster start

# Delete the cluster entirely
sikifanso cluster delete
```

## Next steps

- [Multi-Cluster](guides/multi-cluster.md) — run multiple independent clusters
- [Custom Bootstrap Repos](guides/custom-bootstrap.md) — use your own bootstrap template
- [Architecture](architecture.md) — understand how it all fits together
- [CLI Reference](cli.md) — full list of commands and flags
