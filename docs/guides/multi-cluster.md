# Multi-Cluster

sikifanso supports running multiple independent clusters simultaneously. Each cluster gets its own ports, kubeconfig context, gitops repo, and session metadata.

## Creating multiple clusters

```bash
sikifanso cluster create --name lab1
sikifanso cluster create --name lab2
```

Each cluster is fully independent. They run on different ports and have separate gitops repos.

## Port allocation

Default ports are:

- **30080** — ArgoCD UI
- **30081** — Hubble UI

If the first cluster takes the defaults, the second cluster automatically gets free ports. You can see the assigned ports with:

```bash
sikifanso cluster info lab1
sikifanso cluster info lab2
```

## Targeting a specific cluster

Use the `--cluster` (or `-c`) flag to target commands at a specific cluster:

```bash
# Sync a specific cluster
sikifanso argocd sync --cluster lab1
sikifanso argocd sync --cluster lab2

# Stop / start a specific cluster
sikifanso cluster stop lab1
sikifanso cluster start lab1
```

You can also set the `SIKIFANSO_CLUSTER` environment variable to avoid passing the flag every time:

```bash
export SIKIFANSO_CLUSTER=lab1
sikifanso argocd sync
```

## Listing all clusters

Omit the cluster name from `cluster info` to see all clusters:

```bash
sikifanso cluster info
```

## Independent gitops repos

Each cluster has its own gitops repo at:

```
~/.sikifanso/clusters/<name>/gitops/
```

Deploy different apps to different clusters by managing each gitops repo independently:

```bash
# Deploy podinfo to lab1
mkdir -p ~/.sikifanso/clusters/lab1/gitops/apps/podinfo
cat > ~/.sikifanso/clusters/lab1/gitops/apps/podinfo/config.yaml <<EOF
name: podinfo
repoURL: https://stefanprodan.github.io/podinfo
chart: podinfo
targetRevision: 6.10.1
namespace: podinfo
EOF

cd ~/.sikifanso/clusters/lab1/gitops
git add . && git commit -m "add podinfo"
sikifanso argocd sync --cluster lab1
```

## Deleting a specific cluster

```bash
sikifanso cluster delete lab1
```

This removes the k3d cluster, kubeconfig context, and all associated data for that cluster only. Other clusters are unaffected.
