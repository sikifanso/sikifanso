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

## Health diagnostics -- shipped

`sikifanso doctor` runs a series of health checks against the cluster: Docker daemon, k3d nodes, Cilium, Hubble, ArgoCD, and every enabled catalog app. Each failure includes the root cause and a suggested fix command. See the [CLI reference](cli.md#doctor) for details.

## App catalog -- shipped

The curated app catalog is now available via `sikifanso catalog list/enable/disable`. The bootstrap repo includes 20+ pre-defined apps across monitoring, media, homelab, and dev categories. Enable any catalog app with `sikifanso catalog enable <name>` -- no need to look up repo URLs or chart names. Custom Helm charts can still be deployed via `sikifanso app add`.

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
