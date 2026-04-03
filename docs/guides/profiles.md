# Profiles

Profiles are named presets that enable a curated set of catalog apps in one step. Instead of enabling tools individually, choose a profile that matches your workload and get a working stack immediately.

## Using a profile

Pass `--profile` when creating a cluster:

```bash
sikifanso cluster create --profile agent-dev
```

This creates the cluster and enables the profile's apps automatically. You can also compose multiple profiles:

```bash
sikifanso cluster create --profile agent-dev,rag
```

Composition takes the union of both app sets -- no duplicates.

## Available profiles

List profiles and their included apps:

```bash
sikifanso cluster profiles
```

| Profile | Description | Apps |
|---------|-------------|------|
| `agent-minimal` | Bare minimum to route and observe LLM calls | litellm-proxy, langfuse, postgresql |
| `agent-full` | All AI agent infrastructure tools enabled | All 17 catalog tools |
| `agent-dev` | Local development loop with LLM, RAG, and observability | litellm-proxy, ollama, langfuse, qdrant, postgresql, valkey |
| `agent-safe` | Development stack with all guardrails and policy enforcement | Everything in agent-dev + guardrails-ai, nemo-guardrails, presidio, opa |
| `rag` | RAG-focused stack with vector DB, embeddings, and document parsing | qdrant, text-embeddings-inference, unstructured, postgresql |

## Choosing a profile

**Just getting started?** Use `agent-minimal`. It gives you an LLM gateway (LiteLLM) and tracing (Langfuse) -- enough to route and observe LLM calls without resource overhead.

**Local development with a model?** Use `agent-dev`. Adds Ollama for local inference, Qdrant for vector storage, and Valkey for caching.

**Need safety rails?** Use `agent-safe`. Adds Guardrails AI, NeMo Guardrails, Presidio (PII redaction), and OPA (policy engine) on top of the dev stack.

**Building RAG pipelines?** Compose with the `rag` profile: `--profile agent-dev,rag`. This adds document parsing (Unstructured) and embedding generation (TEI) to your dev stack.

**Want everything?** Use `agent-full`. Enables all 17 catalog tools -- gateway, observability, guardrails, RAG, runtime, models, and storage.

## Adding apps after creation

Profiles set the initial state. You can enable or disable individual apps afterward:

```bash
# Enable an additional tool
sikifanso app enable tempo

# Disable something you don't need
sikifanso app disable ollama
```

## Applying a profile to an existing cluster

Profiles can also be applied via the MCP server's `profile_apply` tool, which enables a profile's apps on a running cluster and triggers an ArgoCD sync.
