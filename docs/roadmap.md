# Roadmap

Ideas and possibilities for future development. Nothing here is guaranteed — these are directions worth exploring.

## Remote GitOps repos

Currently the gitops repo lives on your local filesystem. A natural evolution is supporting **remote git repositories** (e.g., on GitHub) as the gitops source. This would enable:

- Pushing gitops changes from any machine
- Collaborating on cluster configuration
- Using GitHub Actions or other CI to validate app definitions
- Standard pull request workflows for cluster changes

See [Remote GitOps](guides/remote-gitops.md) for more details.

## Additional CNI options

Cilium is a great default, but some users may prefer alternatives:

- **Calico** — widely adopted, simpler configuration
- **Flannel** — lightweight, built into k3s
- **None** — bring your own CNI

A `--cni` flag on `cluster create` could let users choose.

## App marketplace

A curated catalog of pre-built `config.yaml` definitions for popular Helm charts:

- Prometheus + Grafana monitoring stack
- Cert-manager
- Ingress-nginx
- Sealed Secrets
- Loki for log aggregation

Users could deploy from the catalog with a single command like `sikifanso app add prometheus`.

## Cluster templates

Beyond bootstrap repos, full cluster templates that define:

- Node count and resource limits
- Pre-installed apps
- Network policies
- Storage classes

Templates could be shared and reused across teams.

## Terraform / OpenTofu integration

For users who want to manage their homelab infrastructure as code alongside cloud resources. A Terraform provider or module that wraps sikifanso.

## Multi-node topologies

Currently fixed at 1 server + 2 agents. Possibilities:

- Configurable node counts
- HA control plane (3 servers)
- Dedicated nodes for specific workloads (labeled/tainted)

## Backup and restore

Snapshot a cluster's state (gitops repo + session metadata) and restore it later or on a different machine.
