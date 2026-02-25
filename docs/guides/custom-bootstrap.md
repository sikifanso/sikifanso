# Custom Bootstrap Repos

By default, sikifanso scaffolds the gitops repo from the [sikifanso-homelab-bootstrap](https://github.com/sikifanso/sikifanso-homelab-bootstrap) template. You can use your own bootstrap repo to customize what gets deployed out of the box.

## Using a custom bootstrap repo

Pass the `--bootstrap` flag when creating a cluster:

```bash
sikifanso cluster create --name mylab --bootstrap https://github.com/your-org/your-bootstrap.git
```

Or omit the flag and enter the URL when prompted interactively.

## Bootstrap repo structure

A bootstrap repo must follow this structure:

```
your-bootstrap-repo/
├── bootstrap/
│   └── root-app.yaml         # Root ApplicationSet manifest
└── apps/
    ├── coordinates/
    │   └── <app>.yaml         # Helm chart coordinates (optional, pre-installed apps)
    └── values/
        └── <app>.yaml         # Helm values overrides (optional)
```

### root-app.yaml

This is the root `ApplicationSet` that tells ArgoCD how to discover and deploy apps. It uses the git file generator to watch `apps/coordinates/*.yaml`.

The default bootstrap template's root-app.yaml works with any set of apps — you typically don't need to modify it unless you want to change the generator pattern or add additional ApplicationSet features.

### Pre-installed apps

Any apps you include under `apps/coordinates/` in the bootstrap repo will be deployed automatically when the cluster is created. For example, a bootstrap repo with:

```
apps/
├── coordinates/
│   ├── prometheus.yaml
│   └── grafana.yaml
└── values/
    ├── prometheus.yaml
    └── grafana.yaml
```

Would have Prometheus and Grafana deployed alongside ArgoCD and Cilium from the start.

### Coordinate file format

Each app's coordinate file at `apps/coordinates/<name>.yaml` defines a Helm chart source:

```yaml
name: podinfo
repoURL: https://stefanprodan.github.io/podinfo
chart: podinfo
targetRevision: 6.10.1
namespace: podinfo
```

| Field | Description |
|-------|-------------|
| `name` | Application name in ArgoCD |
| `repoURL` | Helm chart repository URL |
| `chart` | Chart name within the repository |
| `targetRevision` | Chart version |
| `namespace` | Kubernetes namespace to deploy into |

The matching values file at `apps/values/<name>.yaml` contains Helm values overrides. It can be empty or a stub — the ApplicationSet references it automatically.

## Creating your own bootstrap repo

1. Fork or clone the [default bootstrap repo](https://github.com/sikifanso/sikifanso-homelab-bootstrap)
2. Add your desired apps under `apps/`
3. Push to your own git hosting
4. Use the `--bootstrap` flag to point at your repo

This lets you create reproducible cluster configurations — every `cluster create` with your bootstrap repo starts with the same set of apps pre-installed.
