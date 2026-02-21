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

## Deploy an app

Apps are defined as Helm chart references in your gitops repo. Let's deploy [podinfo](https://github.com/stefanprodan/podinfo) as an example.

### 1. Create the app directory

```bash
mkdir -p ~/.sikifanso/clusters/default/gitops/apps/podinfo
```

### 2. Add a config.yaml

```bash
cat > ~/.sikifanso/clusters/default/gitops/apps/podinfo/config.yaml <<EOF
name: podinfo
repoURL: https://stefanprodan.github.io/podinfo
chart: podinfo
targetRevision: 6.10.1
namespace: podinfo
EOF
```

### 3. Commit the change

```bash
cd ~/.sikifanso/clusters/default/gitops
git add . && git commit -m "add podinfo"
```

### 4. Sync

ArgoCD picks up changes within ~3 minutes automatically. To force immediate sync:

```bash
sikifanso argocd sync
```

### 5. Verify

```bash
kubectl get applications -n argocd
```

```
NAME      SYNC STATUS   HEALTH STATUS
argocd    Synced        Healthy
cilium    Synced        Healthy
podinfo   Synced        Healthy
```

## Remove an app

Delete the app directory from your gitops repo, commit, and sync:

```bash
rm -rf ~/.sikifanso/clusters/default/gitops/apps/podinfo
cd ~/.sikifanso/clusters/default/gitops
git add . && git commit -m "remove podinfo"
sikifanso argocd sync
```

## Manage your cluster

```bash
# Show cluster details
sikifanso cluster info

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
