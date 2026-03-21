# sikifanso: AI Agent Infrastructure Platform - Design Spec

**Date:** 2026-03-20
**Status:** Active

## Context

sikifanso today is a one-command homelab K8s bootstrapper (k3d + Cilium + ArgoCD + app catalog). The pivot: **replace the homelab catalog with an AI agent infrastructure catalog**, turning sikifanso into a tool that bootstraps Kubernetes clusters purpose-built for running AI agents safely.

**Why this matters:** Running AI agents in production has the same infrastructure chaos that running microservices had in 2016. Everyone builds bespoke stacks. A platform engineer who packages the cross-cutting concerns (gateway, guardrails, observability, isolation, RAG) into a turnkey K8s stack is building the "early Kubernetes" of agent infrastructure.

**What stays:** k3d, Cilium, ArgoCD, the gitops model, the catalog system, the CLI framework, doctor, snapshots, TUI. The *machinery* is sound -- only the *catalog contents* and *identity* change.

**What changes:** Catalog entries swap from homelab apps to AI infra tools. New capabilities added: MCP server interface, AG-UI protocol, agent isolation policies. New profiles for agent workloads.

**Deployment target:** Local-first (k3d on Docker), expandable to cloud K8s clusters in future phases.

---

## Architecture Overview

```
sikifanso cluster create --profile agent-full
    |
    +-- k3d cluster (unchanged)
    +-- Cilium CNI + NetworkPolicies (agent isolation)
    +-- ArgoCD GitOps (unchanged)
    |
    +-- AI Agent Infrastructure Catalog
        +-- Gateway:      LiteLLM Proxy (routing, cost, rate limiting, key mgmt)
        +-- Observe:      Langfuse (LLM tracing), Prometheus+Grafana (infra), Loki (logs)
        +-- Guardrails:   Guardrails AI, NeMo Guardrails, Presidio (PII)
        +-- RAG:          Qdrant (vectors), TEI (embeddings), Unstructured (parsing)
        +-- Runtime:      Temporal (orchestration), External Secrets, OPA (policy)
        +-- Models:       Ollama (local inference)
        +-- Storage:      PostgreSQL, Redis (supporting infra)
        +-- Isolation:    gVisor sandboxes, Cilium L7 egress, namespace-per-agent

New Interfaces:
    +-- MCP Server:  Expose cluster ops + RAG as MCP tools for any agent
    +-- AG-UI:       Stream agent actions to terminal/web UI via SSE
```

---

## Catalog Replacement Map

### Remove (homelab apps)
falco, kyverno, sealed-secrets, trivy-operator, keda, headlamp, mailpit, rabbitmq, cert-manager, metrics-server, reloader, otel-collector

### Keep (reusable infra)
prometheus-stack, grafana (via prometheus-stack), loki, tempo, postgresql, valkey (redis)

### Add (AI agent infra)

| Entry | Category | What It Does | Helm Chart |
|-------|----------|-------------|------------|
| `litellm-proxy` | gateway | LLM API gateway, multi-provider routing, cost tracking, rate limiting | litellm/litellm |
| `langfuse` | observability | LLM tracing, eval tracking, prompt management, cost-per-trace | langfuse/langfuse |
| `guardrails-ai` | guardrails | Output validation, schema enforcement, content filtering | custom/guardrails-ai |
| `nemo-guardrails` | guardrails | Conversational safety rails, topic control, jailbreak prevention | custom/nemo-guardrails |
| `presidio` | guardrails | PII detection and redaction (Microsoft) | custom/presidio |
| `qdrant` | rag | Vector database for embeddings | qdrant/qdrant |
| `text-embeddings-inference` | rag | HuggingFace embedding service | custom/tei |
| `unstructured` | rag | Document parsing, chunking (PDF, HTML, code) | custom/unstructured |
| `temporal` | runtime | Workflow orchestration, retries, multi-agent coordination | temporalio/temporal |
| `external-secrets` | runtime | Secrets management (Vault, AWS SM, etc.) | external-secrets/external-secrets |
| `opa` | runtime | Policy engine for agent RBAC, tool-level access control | opa/opa |
| `ollama` | models | Local LLM inference, model management | otwld/ollama-helm |

### New Categories
`gateway`, `guardrails`, `rag`, `runtime`, `models` (added to existing `observability`, `storage`)

---

## Execution Phases

### Phase 0: Foundation & Identity
Swap catalog contents, update project identity. No new Go code.

### Phase 1: Agent Cluster Profiles
Define opinionated profiles for common agent workloads (agent-minimal, agent-full, agent-dev, agent-safe, rag).

### Phase 2: Agent Isolation & Network Policies
Namespace-per-agent pattern, Cilium NetworkPolicies, gVisor RuntimeClass, `sikifanso agent` commands.

### Phase 3: MCP Server Interface
Expose cluster ops + RAG as MCP tools via `sikifanso mcp serve`.

### Phase 4: AG-UI Protocol Integration
Stream agent reasoning and actions to terminal/web UI in real-time.

### Phase 5: AI-Powered Operations
`sikifanso ai` commands powered by MCP tools and AG-UI streaming.

### Phase 6: Advanced Features
Production hardening, community catalog, multi-cluster federation, GPU scheduling, cloud support.

---

## Phase Dependencies

```
Phase 0 (Foundation)
  +---> Phase 1 (Profiles)
  +---> Phase 2 (Isolation)
           +---> Phase 3 (MCP Server)
                    +---> Phase 4 (AG-UI)
                             +---> Phase 5 (AI Ops)
                                      +---> Phase 6 (Advanced)
```
