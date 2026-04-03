# Getting Started

This guide walks you through creating your first cluster, deploying AI infra tools, creating agent sandboxes, and managing apps.

## Create a cluster

The fastest way to get started is with a profile:

```bash
sikifanso cluster create --profile agent-dev
```

This creates a k3d cluster and enables the `agent-dev` tool set: LiteLLM Proxy, Ollama, Langfuse, Qdrant, PostgreSQL, and Valkey.

You can also create a bare cluster and enable tools individually:

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

## Enable tools from the catalog

Browse all available AI infra tools:

```bash
sikifanso app list --all
```

Enable a tool:

```bash
sikifanso app enable litellm-proxy
```

This sets `enabled: true` in the catalog entry, commits the change, and triggers an ArgoCD sync.

Disable a tool:

```bash
sikifanso app disable litellm-proxy
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

This writes the chart coordinates and a stub values file to your gitops repo, auto-commits, and triggers an ArgoCD sync.

Running `sikifanso app add` with no arguments and no flags launches a **TUI catalog browser** where you can toggle catalog apps interactively. If you provide a name but omit flags, the CLI prompts for each missing field.

## List installed apps

```bash
sikifanso app list
```

```
NAME                 CHART                  VERSION    NAMESPACE    SOURCE
litellm-proxy        litellm                0.2.1      gateway      catalog
langfuse             langfuse               1.2.14     observability catalog
podinfo              podinfo                6.10.1     podinfo      custom
```

Both custom apps and enabled catalog apps are shown, with a `SOURCE` column to distinguish them.

## Remove an app

For custom apps:

```bash
sikifanso app remove podinfo
```

For catalog apps:

```bash
sikifanso app disable litellm-proxy
```

## Create an agent sandbox

Agent sandboxes are isolated namespaces for running AI agent code safely:

```bash
sikifanso agent create my-agent --cpu 1 --memory 1Gi
```

Each sandbox gets resource quotas, network policies (default-deny egress, allowlisted access to LiteLLM, Qdrant, PostgreSQL, Valkey), and its own service account.

```bash
# List agents
sikifanso agent list

# Delete an agent
sikifanso agent delete my-agent
```

## Check cluster health

Run `cluster doctor` to verify that all components are healthy:

```bash
sikifanso cluster doctor
```

This checks Docker, k3d nodes, Cilium, Hubble, ArgoCD, every enabled catalog app, and agent namespaces. If anything is wrong, it tells you exactly what failed and how to fix it.

## Manage your cluster

```bash
# Show cluster details
sikifanso cluster info

# Trigger ArgoCD sync
sikifanso app sync

# Check app status
sikifanso app status

# Stop the cluster (preserves state)
sikifanso cluster stop

# Start it again
sikifanso cluster start

# Delete the cluster entirely
sikifanso cluster delete

# Take a snapshot of your cluster config
sikifanso snapshot capture --name my-backup

# Restore from a snapshot
sikifanso snapshot restore my-backup

# Start the web dashboard
sikifanso cluster dashboard
```

## Next steps

- [Profiles](guides/profiles.md) -- predefined tool sets for common workloads
- [Agent Sandboxes](guides/agent-sandboxes.md) -- isolated namespaces for AI agents
- [MCP Server](guides/mcp-server.md) -- expose operations as MCP tools for AI agents
- [Multi-Cluster](guides/multi-cluster.md) -- run multiple independent clusters
- [Custom Bootstrap Repos](guides/custom-bootstrap.md) -- use your own bootstrap template
- [Architecture](architecture.md) -- understand how it all fits together
- [CLI Reference](cli.md) -- full list of commands and flags
- [Roadmap](roadmap.md) -- what's shipped and what's next
