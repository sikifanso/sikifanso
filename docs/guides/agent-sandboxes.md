# Agent Sandboxes

Agent sandboxes provide isolated Kubernetes namespaces for running untrusted AI agent code. Each sandbox gets its own resource quotas, network policies, and service account.

## Creating an agent

```bash
sikifanso agent create my-agent
```

This scaffolds:

- A dedicated namespace (`agent-my-agent`)
- A `ResourceQuota` limiting CPU, memory, and pod count
- `NetworkPolicy` rules restricting what the agent can reach
- A `ServiceAccount` for the agent workload

The agent definition is written to the gitops repo and deployed via ArgoCD using the [sikifanso-agent-template](https://github.com/sikifanso/sikifanso-agent-template) Helm chart.

## Resource quotas

Customize resource limits with flags:

```bash
sikifanso agent create my-agent --cpu 1 --memory 1Gi --pods 20
```

| Flag | Default | Description |
|------|---------|-------------|
| `--cpu` | `500m` | CPU quota for the namespace |
| `--memory` | `512Mi` | Memory quota for the namespace |
| `--pods` | `10` | Maximum number of pods |

## Listing agents

```bash
sikifanso agent list
```

Shows all agent namespaces with their resource quotas.

## Deleting an agent

```bash
sikifanso agent delete my-agent
```

Removes the agent definition from the gitops repo, commits, and triggers an ArgoCD sync to clean up the namespace and all its resources.

## Network isolation

Agent sandboxes are designed to limit blast radius. Cilium NetworkPolicies enforce:

- **Default deny egress** -- agents cannot reach arbitrary endpoints
- **Allowlisted services** -- agents can reach LiteLLM Proxy (LLM gateway), Qdrant (vector DB), PostgreSQL, and Valkey (cache)
- **No cross-agent traffic** -- agents in different sandboxes cannot communicate
- **No Kubernetes API access** -- agents cannot interact with the cluster control plane

This ensures that even if agent code is compromised, it can only reach the data layer through controlled gateways.

## How it works under the hood

Agent creation writes two files to the gitops repo:

```
gitops/
  agents/
    my-agent.yaml              # Agent definition (name, quotas)
  agents/values/
    my-agent.yaml              # Helm values for the agent-template chart
```

The `root-app.yaml` ApplicationSet picks up the new files and creates an ArgoCD Application that deploys the agent-template Helm chart with the specified values.

## Health checks

`sikifanso cluster doctor` includes agent health checks. It verifies that each agent's ArgoCD Application is Synced and Healthy, and reports any issues with resource quota enforcement or namespace status.
