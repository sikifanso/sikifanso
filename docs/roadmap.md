# Roadmap

sikifanso is an AI agent infrastructure platform. Development follows a phased approach, building from cluster foundations toward AI-powered operations.

---

## Phase 0: Foundation & Identity -- shipped

Replaced the original homelab catalog with AI agent infrastructure tools. 17 curated tools across 7 categories: gateway, observability, guardrails, RAG, runtime, models, and storage. Retained the existing k3d bootstrapping, GitOps catalog system, doctor checks, snapshots, TUI browser, dashboard, and dual-track app model.

## Phase 1: Agent Cluster Profiles -- shipped

`sikifanso cluster create --profile <name>` enables a pre-defined set of catalog apps. Profiles are composable: `--profile agent-dev,rag` enables the union of both. Available profiles: `agent-minimal`, `agent-full`, `agent-dev`, `agent-safe`, `rag`.

See [Profiles](guides/profiles.md) for details.

## Phase 2: Agent Isolation & Network Policies -- shipped

`sikifanso agent create <name>` scaffolds a namespace with resource quotas, network policies, and service account. Cilium NetworkPolicies enforce default-deny egress, allowlisted data layer access, no cross-agent traffic, and no Kubernetes API access.

See [Agent Sandboxes](guides/agent-sandboxes.md) for details.

## Phase 3: MCP Server Interface -- shipped

`sikifanso mcp serve` exposes cluster operations as MCP tools (stdio transport). 22 tools across cluster management, catalog, agents, ArgoCD, Kubernetes, and health checks. Any MCP-compatible client can manage the cluster.

See [MCP Server](guides/mcp-server.md) for details.

---

## Phase 4: AG-UI Protocol Integration

*Stream agent reasoning and actions to terminal/web UI in real-time.*

- AG-UI SSE endpoint in the dashboard server
- Event flow: `RUN_STARTED` -> `STEP_STARTED/FINISHED` -> `TOOL_CALL_*` -> `TEXT_MESSAGE_*` -> `STATE_SNAPSHOT/DELTA`
- Terminal renderer for structured output
- Web dashboard live activity feed
- `sikifanso agent watch <name>` for live agent activity streaming

## Phase 5: AI-Powered Operations

*Natural language cluster operations powered by MCP tools and AG-UI streaming.*

- `sikifanso ai "<prompt>"` -- natural language cluster operations via Claude API with tool use
- AI uses the same MCP tools that external agents use (Phase 3)
- Reasoning streamed via AG-UI (Phase 4)
- Intelligent troubleshooting: `sikifanso ai "why is langfuse unhealthy"`
- AI-driven catalog: `sikifanso ai "I need to run agents with guardrails and cost tracking"`

## Phase 6: Advanced Features

*Production hardening, community, ecosystem.*

- Shareable agent cluster profiles (OCI artifacts / git refs)
- Community catalog contributions (curated AI tool definitions)
- Multi-cluster agent federation
- GPU scheduling support (Ollama/vLLM with NVIDIA GPUs)
- Cloud cluster support (bootstrap EKS/GKE with the same catalog)
- Agent marketplace: pre-built agent templates that run on sikifanso clusters

---

## Phase dependencies

```
Phase 0 (Foundation) -- DONE
  +---> Phase 1 (Profiles) -- DONE
  +---> Phase 2 (Isolation) -- DONE
           +---> Phase 3 (MCP Server) -- DONE
                    +---> Phase 4 (AG-UI)
                             +---> Phase 5 (AI Ops)
                                      +---> Phase 6 (Advanced)
```

## Other ideas

These are directions worth exploring, not committed to any phase:

- **Remote GitOps repos** -- support remote git repos as the gitops source for collaboration and CI/CD. See [Remote GitOps](guides/remote-gitops.md).
- **Additional CNI options** -- Calico, Flannel, or bring-your-own via a `--cni` flag.
- **Cluster templates** -- shareable templates defining node count, resource limits, pre-installed apps, and network policies.
- **Multi-node topologies** -- HA control plane, agent nodes for workload isolation.
- **Terraform / OpenTofu integration** -- a provider or module that wraps sikifanso.
