# MCP Server

sikifanso includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that exposes cluster operations as tools for AI agents. Any MCP-compatible client (Claude Code, Claude Desktop, Cursor, etc.) can manage your cluster through natural language.

## Starting the server

```bash
sikifanso mcp serve
```

The server runs on **stdio transport** -- it reads JSON-RPC from stdin and writes responses to stdout. MCP clients launch this command as a subprocess and communicate over the pipe.

## Configuring with Claude Code

Add to your Claude Code MCP settings:

```json
{
  "mcpServers": {
    "sikifanso": {
      "command": "sikifanso",
      "args": ["mcp", "serve"]
    }
  }
}
```

Then you can ask Claude things like:

- "Create a cluster with the agent-dev profile"
- "Enable langfuse on my default cluster"
- "What's the health status of my cluster?"
- "Create an agent sandbox called research-bot with 1Gi memory"
- "Show me the logs for the litellm-proxy pod"

## Available tools

The MCP server exposes 25 tools across 6 categories:

### Cluster management

| Tool | Description |
|------|-------------|
| `cluster_list` | List all clusters with their state |
| `cluster_info` | Get cluster details (state, services, config) |
| `cluster_create` | Create a new cluster (optionally with a profile) |
| `cluster_delete` | Delete a cluster permanently |
| `cluster_start_stop` | Start or stop a cluster |

### Catalog and profiles

| Tool | Description |
|------|-------------|
| `catalog_list` | List catalog entries with enabled/disabled status |
| `catalog_enable` | Enable a catalog app and sync |
| `catalog_disable` | Disable a catalog app and sync |
| `profile_list` | List available profiles with their apps |
| `profile_apply` | Apply a profile to a running cluster |

### Agent sandboxes

| Tool | Description |
|------|-------------|
| `agent_list` | List agents with resource quotas |
| `agent_info` | Get details about a specific agent |
| `agent_create` | Create an isolated agent namespace |
| `agent_delete` | Delete an agent |

### ArgoCD

| Tool | Description |
|------|-------------|
| `argocd_apps` | List ArgoCD applications with sync/health status |
| `argocd_app_detail` | Get detailed status and resource tree for an app |
| `argocd_app_diff` | Show diff between live and desired state |
| `argocd_rollback` | Roll back an app to a previous revision |
| `argocd_projects_list` | List ArgoCD projects |
| `argocd_project_detail` | Get project details |

### Kubernetes

| Tool | Description |
|------|-------------|
| `kube_pods` | List pods in a namespace |
| `kube_services` | List services in a namespace |
| `kube_logs` | Get recent log lines from a pod |
| `kube_events` | Get recent events in a namespace |

### Health

| Tool | Description |
|------|-------------|
| `doctor` | Run health checks (Docker, nodes, Cilium, ArgoCD, apps, agents) |

## Safety model

All MCP tool calls operate through the same code paths as the CLI. There are no elevated privileges or bypassed checks. Agent-scoped tools respect namespace isolation -- an MCP client cannot access resources outside its designated scope.
