# App Workloads Audit — sikifanso Homelab Bootstrap

**Date**: 2026-04-04
**Scope**: All 19 catalog apps in `sikifanso-homelab-bootstrap/catalog/`
**Cluster type**: Single-node k3d homelab, no GPU assumed unless noted

---

## Shared-Services Audit

| App | Bundles PostgreSQL | Bundles Redis | Bundles S3/MinIO | Bundles ClickHouse | Bundles Other | Can Use Shared? |
|-----|--------------------|---------------|------------------|--------------------|---------------|-----------------|
| alloy | No | No | No | No | No | N/A |
| cnpg-operator | No | No | No | No | No | N/A |
| external-secrets | No | No | No | No | No | N/A |
| guardrails-ai | No | No | No | No | No | N/A |
| langfuse | **deploy: false** (wired to shared) | **deploy: false** (wired to shared) | **Yes — deploy: true** (bundled MinIO) | **Yes — deploy: true** (bundled ClickHouse) | No | PostgreSQL + Valkey: already shared. MinIO + ClickHouse: bundled, no standalone catalog entry yet |
| litellm-proxy | No | No | No | No | No | N/A — but has implicit runtime dep on Langfuse + Ollama |
| loki | No | No | **minio.enabled: false** (explicitly disabled, filesystem used) | No | No | N/A |
| nemo-guardrails | No | No | No | No | No | N/A |
| ollama | No | No | No | No | No | N/A |
| opa | No | No | No | No | No | N/A |
| postgresql | No (IS the shared PostgreSQL) | No | No | No | No | N/A |
| presidio | No | No | No | No | No | N/A |
| prometheus-stack | No | No | No | No | No | N/A |
| qdrant | No | No | No | No | No | N/A |
| temporal | **postgresql.enabled: false** (wired to shared) | No | No | No | cassandra.enabled: false, mysql.enabled: false | PostgreSQL: already shared. No Redis dep. |
| tempo | No | No | No | No | No | N/A |
| text-embeddings-inference | No | No | No | No | No | N/A |
| unstructured | No | No | No | No | No | N/A |
| valkey | No | No | No | No | No | N/A (IS the shared Valkey) |

---

## Executive Summary

Seven of the 19 catalog apps are pure infrastructure services (cnpg-operator, postgresql, valkey, prometheus-stack, external-secrets, opa, alloy) with no bundled dependencies. The main shared-service concern is langfuse, which bundles both ClickHouse and MinIO as internal subcharts — together these add approximately 1–2 GiB of memory overhead on a single-node cluster and neither has a standalone catalog entry. Temporal and langfuse both correctly disable their internal PostgreSQL/Redis subcharts and wire to the shared cluster services. Dependency ordering is partially covered via the `tier`/`dependsOn` mechanism, but a majority of apps (14 of 19) have no `tier` or `dependsOn` field, relying entirely on ArgoCD retry/selfHeal for ordering — this is fragile for apps with hard boot-time database dependencies. The agent sandbox ResourceQuota defaults (500m CPU / 512Mi memory) are appropriate for a lightweight orchestration agent but are too small for any agent that drives local inference inline.

---

## Finding 1: Langfuse Bundles ClickHouse — Significant Memory Overhead, No Shared Alternative

**Severity**: High
**Domain**: AppWorkloads
**Affected app**: langfuse
**Root cause hypothesis**: Langfuse v3 requires ClickHouse for its analytics pipeline (event ingestion, usage tracking). There is no standalone `clickhouse` catalog entry, so the chart deploys a bundled single-node ClickHouse instance. The `langfuse.yaml` values file acknowledges this with the comment "required for Langfuse v3 analytics".
**Evidence**:
- `catalog/values/langfuse.yaml` lines 37–41: `clickhouse.deploy: true` with `auth.password: changeme`
- ClickHouse single-node minimum memory is ~500 MiB at rest, rising to 1–2 GiB under any analytical query load
- No `clickhouse.yaml` entry exists in `catalog/` — ClickHouse cannot be shared between apps today
**Cross-domain flag**: yes — ClickHouse is a candidate for a new standalone catalog entry (storage tier), and langfuse would benefit from pointing at it
**Investigation direction**: Evaluate adding a `clickhouse` catalog entry (Bitnami or Altinity chart, `tier: 1-data`, `dependsOn: cnpg-operator` not applicable but needs ordering before langfuse). Assess whether Langfuse's ClickHouse usage can be pointed at an external host via `clickhouse.host`/`clickhouse.port` values overrides. Verify memory footprint of `clickhouse/clickhouse` vs `bitnami/clickhouse` on a constrained node.

---

## Finding 2: Langfuse Bundles MinIO — Prevents Future Shared Object Store

**Severity**: High
**Domain**: AppWorkloads
**Affected app**: langfuse
**Root cause hypothesis**: Langfuse v3 requires S3-compatible storage for event blobs and media uploads. There is no standalone `minio` catalog entry, so the chart deploys a bundled MinIO instance. The comment in values acknowledges this as temporary: "until standalone catalog entries exist".
**Evidence**:
- `catalog/values/langfuse.yaml` lines 43–52: `s3.deploy: true`, bucket `langfuse`, with static root password
- `catalog/` directory contains no `minio.yaml` — MinIO is not a shared catalog service
- Static `rootPassword: changeme` comment says "prevents ArgoCD sync loops" — this is a deliberate workaround for ArgoCD reconciling secrets
- MinIO at minimum uses ~128–256 MiB memory; with upload activity it peaks higher
**Cross-domain flag**: yes — other apps (temporal workflow history snapshots, future model artifact storage) would benefit from shared S3-compatible storage
**Investigation direction**: Add a standalone `minio` catalog entry (`tier: 1-data`, after `cnpg-operator`). Langfuse can then point its `s3.*` values at the shared MinIO. Evaluate MinIO Operator vs standalone MinIO chart. Confirm Langfuse Helm chart supports `s3.endpoint` override pointing to an external service.

---

## Finding 3: Majority of Apps Have No Tier or dependsOn — Ordering Relies Entirely on ArgoCD Retry

**Severity**: High
**Domain**: AppWorkloads
**Affected app**: alloy, external-secrets, guardrails-ai, langfuse, litellm-proxy, loki, nemo-guardrails, ollama, opa, presidio, qdrant, tempo, text-embeddings-inference, unstructured
**Root cause hypothesis**: The `root-catalog.yaml` ApplicationSet uses a `RollingSync` strategy with three tiers (`0-operators`, `1-data`, `2-services`). Only apps with a `tier` label participate in ordered rollout. Apps without `tier` are applied in an unordered batch. Apps without `dependsOn` have no ArgoCD sync-wave dependency either. For apps that make runtime TCP connections to postgresql or valkey at boot time (langfuse, litellm-proxy, temporal), a cold-start race is possible.
**Evidence**:
- `bootstrap/root-catalog.yaml` lines 8–25: RollingSync with three explicit tier steps
- `catalog/cnpg-operator.yaml`: `tier: 0-operators` — present
- `catalog/postgresql.yaml`: `tier: 1-data`, `dependsOn: [cnpg-operator, prometheus-stack]` — present
- `catalog/langfuse.yaml`: `tier: 2-services`, `dependsOn: [cnpg-operator, postgresql]` — present
- `catalog/temporal.yaml`: `tier: 2-services`, `dependsOn: [cnpg-operator, postgresql]` — present
- `catalog/valkey.yaml`: **no tier** — valkey will deploy in the unordered batch, potentially after langfuse tries to connect to it
- `catalog/litellm-proxy.yaml`: **no tier, no dependsOn** — litellm-proxy has an env var pointing to Langfuse (`LANGFUSE_HOST`) but no declared dependency
- `catalog/alloy.yaml`: **no tier, no dependsOn** — alloy hardcodes `loki.observability.svc.cluster.local` and `tempo.observability.svc.cluster.local`; if those aren't up yet, alloy config reload will fail silently
- `catalog/loki.yaml`, `catalog/tempo.yaml`: **no tier, no dependsOn** — they are alloy's runtime targets
**Cross-domain flag**: no
**Investigation direction**: Add `tier: 1-data` to `valkey.yaml`. Add `tier: 2-services` and `dependsOn: [valkey]` to `langfuse.yaml` (valkey is already a runtime dep, just not declared). Add `tier: 2-services` and `dependsOn: [langfuse, ollama]` to `litellm-proxy.yaml`. Group loki/tempo into `tier: 1-data` or `tier: 2-services` and add alloy's `dependsOn: [loki, tempo]`.

---

## Finding 4: Temporal Has No dependsOn for valkey / Missing Visibility Store Awareness

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: temporal
**Root cause hypothesis**: Temporal uses PostgreSQL for both its default persistence store and its visibility store. The catalog entry correctly depends on `cnpg-operator` and `postgresql`. However, Temporal's Helm chart (`go.temporal.io/helm-charts`) can optionally use Elasticsearch for advanced visibility; it defaults to SQL visibility, which is correctly wired here. The concern is that `temporal` has no `tier` label, so it may deploy before `postgresql` has finished initializing its `temporal` and `temporal_visibility` databases via `postInitSQL`.
**Evidence**:
- `catalog/temporal.yaml` lines 9–12: `tier: 2-services`, `dependsOn: [cnpg-operator, postgresql]` — tier IS present
- `catalog/values/temporal.yaml` lines 43–55: hardcoded `temporal` role and password matching `postgresql.yaml` postInitSQL
- `catalog/values/temporal.yaml` line 35: `schema.setup.enabled: true` — Temporal will attempt DDL migration at startup; if PostgreSQL hasn't finished the `postInitSQL` grants yet, the migration job will fail
- `catalog/values/temporal.yaml` lines 25–28: `postgresql.enabled: false`, `cassandra.enabled: false` — correctly disables bundled DBs
**Cross-domain flag**: no
**Investigation direction**: The `dependsOn` is present and correct. The risk is the postInitSQL timing window. Add an ArgoCD sync-wave annotation or a Kubernetes initContainer in a custom patch to wait for the `temporal` role to exist before the schema setup job runs. Alternatively, document that the `temporal` ArgoCD app may need one manual retry after fresh cluster creation.

---

## Finding 5: litellm-proxy Has No Declared Dependencies Despite Hard Runtime Coupling

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: litellm-proxy
**Root cause hypothesis**: The litellm-proxy values file hardcodes `LANGFUSE_HOST`, `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY` as env vars, and the `proxy_config.model_list` points to `ollama.models.svc.cluster.local`. At startup litellm-proxy will attempt to reach both Langfuse and Ollama. Neither is in a `dependsOn` list, and there is no `tier` label.
**Evidence**:
- `catalog/litellm-proxy.yaml`: no `tier`, no `dependsOn`
- `catalog/values/litellm-proxy.yaml` lines 37–39: `LANGFUSE_HOST`, `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY`
- `catalog/values/litellm-proxy.yaml` lines 22–24: `ollama/llama3.2:1b` via `http://ollama.models.svc.cluster.local:11434`
- LiteLLM proxy performs a health-check ping to configured backends on startup and will log errors (but does not crash) if they are unreachable — however, the Langfuse callback integration requires Langfuse to be reachable for trace submission
**Cross-domain flag**: no
**Investigation direction**: Add `tier: 2-services` and `dependsOn: [langfuse, ollama]` to `catalog/litellm-proxy.yaml`. Also consider adding a `litellm` database to postgresql's `postInitSQL` (litellm-proxy optionally persists spend/key data to PostgreSQL; the current config does not wire this but could benefit from it).

---

## Finding 6: alloy Has No Declared Dependencies on loki and tempo

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: alloy
**Root cause hypothesis**: Alloy's config (embedded in values) hardcodes two endpoint URLs: `loki.observability.svc.cluster.local:3100` and `tempo.observability.svc.cluster.local:4317`. If alloy starts before loki or tempo, log/trace shipping will fail until those services become available. Alloy does retry, but log loss during the startup window is possible.
**Evidence**:
- `catalog/values/alloy.yaml` lines 40–43: `loki.write.default` endpoint
- `catalog/values/alloy.yaml` lines 60–65: `otelcol.exporter.otlp.tempo` endpoint
- `catalog/alloy.yaml`: no `tier`, no `dependsOn`
- `catalog/loki.yaml`: no `tier`, no `dependsOn`
- `catalog/tempo.yaml`: no `tier`, no `dependsOn`
**Cross-domain flag**: no
**Investigation direction**: Add `tier: 1-data` (or `tier: 2-services`) to `loki.yaml` and `tempo.yaml`, then add `dependsOn: [loki, tempo]` to `alloy.yaml`. Both loki and tempo are pure-filesystem deployments with no upstream dependencies of their own, so placing them in `tier: 1-data` is appropriate.

---

## Finding 7: Ollama CPU-Only Memory Limit May Be Insufficient for llama3.2:1b on Constrained Nodes

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: ollama
**Root cause hypothesis**: The values file sets a 4 GiB memory limit and pulls `llama3.2:1b` on startup. llama3.2:1b in GGUF Q4_K_M quantisation requires approximately 800 MiB of RAM for the model weights. The 4 GiB limit is reasonable headroom, but the 1 GiB *request* means the scheduler may place ollama on a node where only 1 GiB is actually free — the model will then OOM-kill during inference.
**Evidence**:
- `catalog/values/ollama.yaml` lines 11–17: `requests.memory: 1Gi`, `limits.memory: 4Gi`
- `catalog/values/ollama.yaml` lines 7–9: pulls `llama3.2:1b` at startup
- On a single-node k3d cluster the scheduler always places on the one node, so the gap between request and limit is less dangerous but still causes misleading capacity reporting
- llama3.2:1b peak RSS during inference (CPU): ~2–2.5 GiB. The 4 GiB limit is adequate; the 1 GiB request is misleading.
**Cross-domain flag**: no
**Investigation direction**: Raise `requests.memory` to at least `2Gi` to match realistic working set. Consider documenting GPU values override (already commented out in the file). For models larger than 3B params, note that the 4 GiB limit must also be raised.

---

## Finding 8: text-embeddings-inference CPU-Only Throughput Is Very Low; Memory Request Understated

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: text-embeddings-inference
**Root cause hypothesis**: `BAAI/bge-small-en-v1.5` in CPU mode loads approximately 130 MB of model weights. The 512 MiB memory request is adequate for the model itself, but the HuggingFace TEI server also loads tokeniser libraries (~200 MB) and allocates batch buffers. On CPU, throughput is roughly 10–50 sentences/second depending on batch size. This is functionally usable for low-QPS RAG pipelines but will become a bottleneck if more than one agent submits concurrent embedding requests.
**Evidence**:
- `catalog/values/text-embeddings-inference.yaml`: `model: "BAAI/bge-small-en-v1.5"`, `requests.memory: 512Mi`, `limits.memory: 2Gi`
- HuggingFace TEI image does not have a CPU-only tag for this chart version (`0.1.0`); the default image may attempt to load CUDA libs and fall back gracefully, adding startup latency
**Cross-domain flag**: no
**Investigation direction**: Verify that `targetRevision: 0.1.0` of the HuggingFace helm chart supports a `cpu` runtime flag or explicit image tag override for CPU-only inference. Consider bumping to a later chart version that exposes `runtime: cpu` as a value. For higher throughput, evaluate switching to a smaller ONNX-quantized model.

---

## Finding 9: Loki Uses Filesystem Backend — Will Lose Logs on Pod Restart Without Verified PVC

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: loki
**Root cause hypothesis**: Loki is deployed in `SingleBinary` mode with `storage.type: filesystem`. The values file enables persistence (`persistence.enabled: true`, `size: 10Gi`) and explicitly disables MinIO (`minio.enabled: false`). This is appropriate for homelab. However, the single-binary image and its filesystem storage mean that if the PVC is lost (e.g., cluster recreated with `k3d cluster delete`), all log history is gone. There is no S3 fallback path configured.
**Evidence**:
- `catalog/values/loki.yaml` lines 9–10: `storage.type: filesystem`
- `catalog/values/loki.yaml` lines 31–33: `persistence.enabled: true`, `size: 10Gi`
- `catalog/values/loki.yaml` lines 43–44: `minio.enabled: false`
- The Loki chart `6.55.0` defaults to S3/MinIO in distributed mode; the explicit `minio.enabled: false` is correct and intentional for single-binary
**Cross-domain flag**: yes — if a shared MinIO catalog entry is added (see Finding 2), loki could optionally be migrated to object storage backend for durability across cluster rebuilds
**Investigation direction**: This is acceptable for homelab. Document that log history is ephemeral and tied to PVC lifecycle. If durability is required, wire loki to MinIO once the shared MinIO catalog entry exists.

---

## Finding 10: guardrails-ai Memory Limit Is Likely Insufficient for Rule Validation with NLP Models

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: guardrails-ai
**Root cause hypothesis**: The Guardrails AI API server (`guardrails-ai/guardrails-api` chart `0.1.0`) is a Python FastAPI application. On startup it loads whatever validators are installed. The 512 MiB limit is adequate for the base server with simple regex-based validators but will be exceeded if spaCy or transformers-based validators are loaded (each NLP model adds 200–500 MiB).
**Evidence**:
- `catalog/values/guardrails-ai.yaml`: `requests.memory: 256Mi`, `limits.memory: 512Mi`
- Guardrails AI's `guardrails-api` Docker image bundles the server but not validators; validators are installed at runtime via `GUARDRAILS_TOKEN` env + hub pull, or baked into a custom image
- Python process baseline RSS ~100–150 MiB; any spaCy model adds ~200–400 MiB
**Cross-domain flag**: no
**Investigation direction**: If only schema/regex validators are used, current limits are fine. If NLP-backed validators (e.g., `toxic-language`, `detect-pii`) are needed, raise limit to at least `1Gi` or `2Gi`. Clarify which validators are expected to be loaded at runtime and document in the values file.

---

## Finding 11: nemo-guardrails Memory Limit Understated for LLM-Backed Rails

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: nemo-guardrails
**Root cause hypothesis**: NeMo Guardrails is a Python server that invokes an LLM to evaluate safety rules. The 1 GiB memory limit covers only the server process; any actual LLM inference happens via an external endpoint (Ollama or OpenAI API). The limit is fine if NeMo is configured as a pure proxy (forwarding to Ollama). However, if the NeMo Guardrails image bundles a local model or loads sentence-transformers for semantic similarity checks, 1 GiB will be insufficient.
**Evidence**:
- `catalog/values/nemo-guardrails.yaml`: `requests.memory: 512Mi`, `limits.memory: 1Gi`
- NeMo Guardrails `0.1.0` chart does not appear to bundle a local model — it expects an LLM endpoint configuration
- No `llm.endpoint` or `openai.*` wiring is present in the values file — the app will fail to apply any LLM-backed rail without additional configuration
**Cross-domain flag**: yes — nemo-guardrails has an implicit runtime dependency on litellm-proxy or ollama (no endpoint configured in values)
**Investigation direction**: Add LLM endpoint configuration to `catalog/values/nemo-guardrails.yaml` pointing to litellm-proxy (`http://litellm-proxy.gateway.svc.cluster.local:4000`). Add `dependsOn: [litellm-proxy]` to `catalog/nemo-guardrails.yaml`. Verify that `nemo-guardrails/nemo-guardrails` chart `0.1.0` is an official NVIDIA chart and not a community placeholder — the chart repo `https://nvidia.github.io/NeMo-Guardrails` should be verified.

---

## Finding 12: presidio Analyzer Baseline Memory Is Understated for spaCy en_core_web_lg

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: presidio
**Root cause hypothesis**: Microsoft Presidio's analyzer component loads a spaCy NLP model at startup (default: `en_core_web_lg`). This model requires ~750 MiB of RAM. The current values set a 512 MiB limit for the analyzer, which will cause an OOM kill before the model finishes loading.
**Evidence**:
- `catalog/values/presidio.yaml` analyzer: `requests.memory: 256Mi`, `limits.memory: 512Mi`
- Microsoft Presidio Docker image (`mcr.microsoft.com/presidio-analyzer`) bundles `en_core_web_lg` by default
- spaCy `en_core_web_lg` loaded RSS: ~700–800 MiB; `en_core_web_sm` is ~50 MiB
**Cross-domain flag**: no
**Investigation direction**: Raise `analyzer.resources.limits.memory` to at least `1Gi` (or `1.5Gi` to be safe). Alternatively, confirm whether the chart version `0.1.0` allows configuring a smaller spaCy model (`en_core_web_sm`) via a Helm value — if so, `sm` would fit within 512 MiB. The anonymizer (128Mi request / 256Mi limit) is fine as it is purely rule-based.

---

## Finding 13: postgresql ResourceQuota max_connections May Be Exceeded Under Full Profile

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: postgresql
**Root cause hypothesis**: The shared PostgreSQL cluster sets `max_connections: 200`. In the `agent-full` profile, multiple apps open connection pools: langfuse (default pool ~10–20), temporal (default pool ~20–50), litellm-proxy (optional, if wired to PG), plus any agent workloads that connect directly. Under full concurrency, total connections can approach or exceed 200, causing new connection attempts to fail.
**Evidence**:
- `catalog/values/postgresql.yaml` lines 51–53: `max_connections: "200"`, `shared_buffers: "64MB"`
- `catalog/values/temporal.yaml`: wires to `temporal` and `temporal_visibility` databases — Temporal server uses a default pool of 20 connections per DB = 40 total minimum
- `catalog/values/langfuse.yaml`: wires to `langfuse` database — langfuse default pool is 10
- Total static minimum: ~50–60 connections; with agent workloads and spikes, 200 is reachable
- `shared_buffers: 64MB` is below PostgreSQL's recommended minimum of 25% of RAM; with 512 MiB limit, this should be at least 128MB
**Cross-domain flag**: no
**Investigation direction**: Consider adding PgBouncer as a connection pooler (can be deployed as a sidecar or separate catalog entry). Raise `shared_buffers` to `128MB`. Alternatively, raise `max_connections` to 400 and increase `resources.limits.memory` to `1Gi` to accommodate the larger shared memory allocation.

---

## Finding 14: prometheus-stack Has No Declared dependsOn for postgresql Monitoring Integration

**Severity**: Low
**Domain**: AppWorkloads
**Affected app**: prometheus-stack
**Root cause hypothesis**: The `postgresql.yaml` catalog entry declares `dependsOn: [cnpg-operator, prometheus-stack]` because it creates a `PodMonitor` CRD resource that requires prometheus-stack's CRDs. This is correct. However, `prometheus-stack.yaml` itself has `tier: 0-operators` but no explicit `dependsOn` — meaning ArgoCD deploys it in the first tier wave, which is correct. The risk is the reverse: if someone disables prometheus-stack but leaves postgresql enabled, the `podMonitor.enabled: true` in postgresql values will fail to create a PodMonitor because the CRD is gone. This is an operational rather than deployment-time concern.
**Evidence**:
- `catalog/postgresql.yaml` lines 9–12: `tier: 1-data`, `dependsOn: [cnpg-operator, prometheus-stack]`
- `catalog/values/postgresql.yaml` lines 28–30: `monitoring.enabled: true`, `podMonitor.enabled: true`
- `catalog/prometheus-stack.yaml`: `tier: 0-operators` — correct tier, no dependsOn needed
**Cross-domain flag**: no
**Investigation direction**: Document that postgresql monitoring requires prometheus-stack. Optionally add a conditional in postgresql values: if prometheus-stack is not enabled, set `monitoring.enabled: false`. Currently the `dependsOn` prevents this scenario during initial deploy but does not protect against runtime removal of prometheus-stack.

---

## Finding 15: opa Has No dependsOn but Runtime Integration Requires external-secrets for Policy Secrets

**Severity**: Low
**Domain**: AppWorkloads
**Affected app**: opa
**Root cause hypothesis**: OPA (Open Policy Agent with kube-mgmt) loads policies from Kubernetes ConfigMaps/Secrets. If policies reference secrets managed by external-secrets, OPA requires external-secrets to be running and secrets to be synced before policies can be evaluated. There is no `dependsOn: [external-secrets]` in the opa catalog entry.
**Evidence**:
- `catalog/opa.yaml`: no `tier`, no `dependsOn`
- `catalog/external-secrets.yaml`: `tier: 0-operators`
- OPA itself does not fail without external-secrets — it starts fine and loads policies from ConfigMaps. The dependency is only relevant if ExternalSecret resources are the policy source.
**Cross-domain flag**: no
**Investigation direction**: If policy secrets are managed via external-secrets, add `dependsOn: [external-secrets]` to `opa.yaml`. Otherwise this is informational only. Consider adding `tier: 1-data` to opa to ensure it syncs after operators.

---

## Finding 16: qdrant Has No dependsOn but Profiles Imply It Must Start Before Agent Workloads

**Severity**: Low
**Domain**: AppWorkloads
**Affected app**: qdrant
**Root cause hypothesis**: The agent NetworkPolicy template in `sikifanso-agent-template` hardcodes an egress allowlist entry to `rag` namespace port 6333. This means agents are already designed to use qdrant. However, qdrant has no `tier` or `dependsOn` in the catalog, so it may deploy in an unordered wave. Since qdrant is a pure stateful store with no upstream dependencies, the only risk is agents trying to connect before qdrant's PVC is bound and the server is ready.
**Evidence**:
- `catalog/qdrant.yaml`: no `tier`, no `dependsOn`
- `sikifanso-agent-template/values.yaml` lines 22–25: qdrant allowlist entry hardcoded at `rag:6333`
- `profile.go` lines 53–54: rag profile includes qdrant
**Cross-domain flag**: no
**Investigation direction**: Add `tier: 1-data` to `qdrant.yaml` to ensure it deploys in the data tier before service-tier agents start connecting. No `dependsOn` needed as qdrant has no upstream service dependencies.

---

## Finding 17: Agent Sandbox ResourceQuota Conflates Requests and Limits

**Severity**: Medium
**Domain**: AppWorkloads
**Affected app**: N/A (agent-template)
**Root cause hypothesis**: The agent ResourceQuota template sets `requests.cpu`, `requests.memory`, `limits.cpu`, and `limits.memory` all to the same value (`agent.cpu` and `agent.memory`). This forces every container in the agent namespace to set requests == limits (otherwise the quota is exceeded). This is a valid conservative strategy but prevents burstable QoS for agents — every agent pod is forced into `Guaranteed` QoS class, which is unnecessarily restrictive for orchestration agents that have variable CPU usage.
**Evidence**:
- `sikifanso-agent-template/templates/resourcequota.yaml` lines 10–13: `requests.cpu`, `limits.cpu`, `requests.memory`, `limits.memory` all set to identical values
- Default values: `cpu: "500m"`, `memory: "512Mi"` — these are quota totals for the entire namespace, not per-pod limits
- A minimal orchestration agent (ReAct loop calling litellm-proxy) needs ~100–200m CPU at burst and ~256 MiB; the 500m/512Mi quota covers one agent process but leaves no headroom for auxiliary containers (sidecar, init containers)
- A heavy agent (local Ollama-backed inference) would need at least 2000m CPU and 4Gi memory, requiring custom quota override at agent creation time
**Cross-domain flag**: no
**Investigation direction**: Split the ResourceQuota into separate request and limit buckets. For example, `requests.cpu: 500m`, `limits.cpu: 2000m` allows burst. Alternatively, document that `agent.cpu` sets the request ceiling and advise users to pass `--cpu 2` for inference-heavy agents. Add two named presets in the template `ci/test-values.yaml`: `minimal` (500m/512Mi) and `inference` (2000m/8Gi).

---

## Finding 18: agent-minimal Profile Missing valkey Dependency

**Severity**: Low
**Domain**: AppWorkloads
**Affected app**: profile: agent-minimal
**Root cause hypothesis**: The `agent-minimal` profile (`profile.go` line 22–25) enables `[litellm-proxy, langfuse, cnpg-operator, postgresql]`. Langfuse's values file wires to valkey (`redis.host: valkey.storage.svc.cluster.local`) but valkey is not in the agent-minimal app list. When the profile is applied, langfuse will deploy and fail its Redis connection on startup.
**Evidence**:
- `internal/profile/profile.go` lines 22–25: `agent-minimal` apps: `[litellm-proxy, langfuse, cnpg-operator, postgresql]`
- `catalog/values/langfuse.yaml` lines 29–34: `redis.deploy: false`, `redis.host: valkey.storage.svc.cluster.local` — requires valkey to be running
- `catalog/langfuse.yaml` lines 10–12: `dependsOn: [cnpg-operator, postgresql]` — valkey not listed
**Cross-domain flag**: no
**Investigation direction**: Add `valkey` to the `agent-minimal` profile apps list. Also add `dependsOn: [valkey]` to `catalog/langfuse.yaml` to make the dependency explicit in the ArgoCD ApplicationSet ordering.

---

## Finding 19: Temporal schema.setup Job May Conflict with Multiple Restarts on Fresh Cluster

**Severity**: Low
**Domain**: AppWorkloads
**Affected app**: temporal
**Root cause hypothesis**: The Temporal Helm chart's `schema.setup.enabled: true` runs a Kubernetes Job to apply DDL migrations. If Temporal is deployed before PostgreSQL's `postInitSQL` completes (creating the `temporal` role and databases), the Job fails. ArgoCD's retry (limit: 3, backoff: 10s) may exhaust retries before PostgreSQL is ready on a slow node, leaving the cluster in a degraded state that requires manual intervention.
**Evidence**:
- `catalog/values/temporal.yaml` line 35: `schema.setup.enabled: true`
- `catalog/values/postgresql.yaml` lines 44–49: `postInitSQL` creates `temporal` role and databases — this runs after the PostgreSQL cluster is `Ready` but timing relative to ArgoCD sync is not guaranteed
- `catalog/temporal.yaml` lines 9–12: `tier: 2-services`, `dependsOn: [cnpg-operator, postgresql]` — tier is correct but postInitSQL completion is not a Kubernetes readiness signal
- ArgoCD retry: `limit: 3`, `backoff.duration: 10s`, `backoff.maxDuration: 3m` — the 3-minute window may be insufficient if the single-node cluster is under memory pressure during bootstrap
**Cross-domain flag**: no
**Investigation direction**: Add an init container to the temporal schema setup Job (via Helm values or a kustomize patch) that uses `psql -c '\du'` to verify the `temporal` role exists before proceeding. Alternatively, increase ArgoCD retry limit to 5 and `maxDuration` to 10m for the temporal application specifically, using an ArgoCD Application override in the ApplicationSet template.

---

## Appendix: Resource Summary Table

| App | Req CPU | Req Memory | Limit CPU | Limit Memory | Notes |
|-----|---------|------------|-----------|--------------|-------|
| alloy | 50m | 64Mi | 200m | 256Mi | DaemonSet — per node |
| cnpg-operator | (not set in values) | — | — | — | Operator; chart defaults apply |
| external-secrets | 100m total | 128Mi total | 500m total | 512Mi total | 3 components (main+webhook+cert) |
| guardrails-ai | 100m | 256Mi | 500m | 512Mi | May need 1Gi for NLP validators |
| langfuse | 100m | 256Mi | 500m | 1Gi | Plus bundled ClickHouse + MinIO overhead |
| litellm-proxy | 100m | 256Mi | 500m | 512Mi | |
| loki | 50m | 128Mi | 200m | 512Mi | SingleBinary mode |
| nemo-guardrails | 200m | 512Mi | 500m | 1Gi | Needs LLM endpoint configured |
| ollama | 500m | 1Gi | 2000m | 4Gi | req.memory should be 2Gi |
| opa | 100m total | 128Mi total | 400m total | 512Mi total | 2 containers (opa+mgmt) |
| postgresql | 100m | 256Mi | 500m | 512Mi | shared_buffers should be 128MB |
| presidio | 150m | 384Mi | 700m | 768Mi | analyzer limit must be >=1Gi |
| prometheus-stack | ~870m | ~788Mi | ~950m | ~2.8Gi | Sum across 5 components |
| qdrant | 100m | 256Mi | 500m | 1Gi | |
| temporal | 150m | 384Mi | 700m | 1.25Gi | server + web |
| tempo | 50m | 128Mi | 200m | 512Mi | |
| text-embeddings-inference | 200m | 512Mi | 1000m | 2Gi | CPU-only; low throughput |
| unstructured | 200m | 512Mi | 1000m | 2Gi | |
| valkey | 50m | 64Mi | 500m | 256Mi | |

**Approximate total (all enabled, agent-full profile)**: ~3.9 CPU cores requested, ~6.5 GiB memory requested; ~11 CPU cores limit, ~22 GiB memory limit. A single-node k3d host should have at least 8 GiB RAM available to the cluster (16 GiB recommended) to avoid OOM pressure under the agent-full profile.
