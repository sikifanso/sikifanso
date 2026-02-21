# Remote GitOps

!!! note "Future feature"
    This page describes a planned capability that is not yet implemented. See the [Roadmap](../roadmap.md) for other upcoming ideas.

## The idea

Currently, sikifanso uses a **local git repository** on your filesystem as the gitops source. ArgoCD reads from it via a hostPath mount — no remote server needed.

A natural next step is supporting **remote git repositories** (e.g., GitHub, GitLab) as the gitops source. This would change the workflow from local-only to collaborative and cloud-connected.

## What would change

### Current workflow (local)

```
Edit files locally → git commit → sikifanso argocd sync
```

- Gitops repo lives at `~/.sikifanso/clusters/<name>/gitops/`
- ArgoCD reads from a hostPath mount inside the cluster
- Changes are immediate (after commit + sync)

### Future workflow (remote)

```
Edit files anywhere → git push → ArgoCD syncs automatically
```

- Gitops repo hosted on GitHub (or any git provider)
- ArgoCD configured with repo credentials
- Standard ArgoCD polling or webhook-based sync
- No need for `sikifanso argocd sync` — ArgoCD handles it natively

## Benefits

- **Multi-machine access** — push changes from any machine, not just the one running the cluster
- **Collaboration** — multiple people can contribute to cluster configuration
- **Pull request workflows** — review and approve changes before they're applied
- **CI/CD integration** — validate app definitions with GitHub Actions before merging
- **Audit trail** — full git history on a hosted platform
- **Backup** — gitops state is stored remotely, not just on local disk

## Open questions

- How to handle initial repo creation (should sikifanso create the GitHub repo?)
- Authentication (SSH keys, personal access tokens, GitHub App?)
- Should local and remote modes coexist or be mutually exclusive?
- Migration path from local to remote for existing clusters
