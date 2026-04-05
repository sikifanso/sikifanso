# Homelab Resource Findings — sikifanso Catalog

**Date**: 2026-04-04
**Scope**: 19 catalog apps + 5 profiles + agent sandbox template
**Assumption**: Single-node k3d cluster on a developer laptop, 8-16 GB RAM available for Kubernetes workloads.

---

## Resource Summary Table

| App | Memory Request | Memory Limit | CPU Request | CPU Limit | PVC Size | Replicas | Values File? |
|-----|---------------|--------------|-------------|-----------|----------|----------|-------------|
| alloy | 64Mi | 256Mi | 50m | 200m | none | 1 (DaemonSet) | yes |
| cnpg-operator | upstream default | upstream default | upstream default | upstream default | none | 1 | yes (no resources set) |
| external-secrets | 64Mi + 32Mi + 32Mi = 128Mi total | 256Mi + 128Mi + 128Mi = 512Mi total | 100m total | 400m total | none | 1+1+1 | yes |
| guardrails-ai | 256Mi | 512Mi | 100m | 500m | none | 1 | yes |
| langfuse | 256Mi (app only) | 1Gi (app only) | 100m (app only) | 500m (app only) | none (app) | 1 | yes |
| langfuse/clickhouse (bundled) | upstream default | upstream default | upstream default | upstream default | upstream default | upstream default | n/a — sub-chart |
| langfuse/minio (bundled) | upstream default | upstream default | upstream default | upstream default | upstream default | upstream default | n/a — sub-chart |
| litellm-proxy | 256Mi | 512Mi | 100m | 500m | none | 1 | yes |
| loki | 128Mi | 512Mi | 50m | 200m | 10Gi | 1 | yes |
| nemo-guardrails | 512Mi | 1Gi | 200m | 500m | none | 1 | yes |
| ollama | 1Gi | 4Gi | 500m | 2000m | 20Gi | 1 | yes |
| opa | 64Mi + 64Mi = 128Mi total | 256Mi + 256Mi = 512Mi total | 100m total | 400m total | none | 1 | yes |
| postgresql (CNPG) | 256Mi | 512Mi | 100m | 500m | 10Gi | 1 | yes |
| presidio | 256Mi + 128Mi = 384Mi total | 512Mi + 256Mi = 768Mi total | 150m total | 700m total | none | 2 (1+1) | yes |
| prometheus-stack | 752Mi total (see note) | 2.9Gi total (see note) | 320m total | ~1000m total | 20Gi (prometheus) + 5Gi (alertmanager) + 2Gi (grafana) = 27Gi | 1 each | yes |
| qdrant | 256Mi | 1Gi | 100m | 500m | 10Gi | 1 | yes |
| tempo | 128Mi | 512Mi | 50m | 200m | 10Gi | 1 | yes |
| temporal | 256Mi + 128Mi = 384Mi total | 1Gi + 256Mi = 1.25Gi total | 150m total | 700m total | none (uses cluster PG) | 1+1 | yes |
| text-embeddings-inference | 512Mi | 2Gi | 200m | 1000m | none | 1 | yes |
| unstructured | 512Mi | 2Gi | 200m | 1000m | none | 1 | yes |
| valkey | 64Mi | 256Mi | 50m | 500m | 2Gi | 1 | yes |

**Notes on prometheus-stack totals**: prometheus 512Mi + grafana 128Mi + operator 64Mi + kube-state-metrics 32Mi + node-exporter 16Mi = 752Mi requests. Limits: prometheus 2Gi + grafana 512Mi + operator 256Mi + ksm 128Mi + node-exporter 64Mi = 2.96Gi.

**agent-sandbox template**: ResourceQuota hard cap per agent namespace — 500m CPU request/limit, 512Mi memory request/limit, 10 pods. These caps apply to whatever workload runs inside each agent namespace and are sensible for a homelab.

---

## Profile Memory Request Totals

### agent-minimal
Apps: litellm-proxy, langfuse, cnpg-operator, postgresql

| App | Memory Request |
|-----|---------------|
| litellm-proxy | 256Mi |
| langfuse (app) | 256Mi |
| langfuse/clickhouse (bundled, upstream) | ~500Mi (typical upstream default) |
| langfuse/minio (bundled, upstream) | ~128Mi (typical upstream default) |
| cnpg-operator | ~128Mi (upstream default) |
| postgresql | 256Mi |
| **Total** | **~1.5Gi** |

Verdict: Fits comfortably in 8Gi. The unknown is ClickHouse.

### agent-dev
Apps: litellm-proxy, ollama, langfuse, qdrant, cnpg-operator, postgresql, valkey, alloy

| App | Memory Request |
|-----|---------------|
| litellm-proxy | 256Mi |
| ollama | 1Gi |
| langfuse (app) | 256Mi |
| langfuse/clickhouse (bundled) | ~500Mi |
| langfuse/minio (bundled) | ~128Mi |
| qdrant | 256Mi |
| cnpg-operator | ~128Mi |
| postgresql | 256Mi |
| valkey | 64Mi |
| alloy | 64Mi |
| **Total** | **~2.9Gi** |

Verdict: Fits in 8Gi with room to spare. Ollama is the dominant consumer.

### agent-safe
Apps: litellm-proxy, ollama, langfuse, qdrant, cnpg-operator, postgresql, valkey, guardrails-ai, nemo-guardrails, presidio, opa

| App | Memory Request |
|-----|---------------|
| litellm-proxy | 256Mi |
| ollama | 1Gi |
| langfuse (app + bundled) | ~884Mi |
| qdrant | 256Mi |
| cnpg-operator | ~128Mi |
| postgresql | 256Mi |
| valkey | 64Mi |
| guardrails-ai | 256Mi |
| nemo-guardrails | 512Mi |
| presidio (2 pods) | 384Mi |
| opa | 128Mi |
| **Total** | **~4.1Gi** |

Verdict: Fits in 8Gi but tight. Multiple inference/NLP services run concurrently.

### rag
Apps: qdrant, text-embeddings-inference, unstructured, cnpg-operator, postgresql

| App | Memory Request |
|-----|---------------|
| qdrant | 256Mi |
| text-embeddings-inference | 512Mi |
| unstructured | 512Mi |
| cnpg-operator | ~128Mi |
| postgresql | 256Mi |
| **Total** | **~1.7Gi** |

Verdict: Very comfortable.

### agent-full (all 19 apps)

| App | Memory Request |
|-----|---------------|
| alloy | 64Mi |
| cnpg-operator | ~128Mi |
| external-secrets | 128Mi |
| guardrails-ai | 256Mi |
| langfuse (app + bundled) | ~884Mi |
| litellm-proxy | 256Mi |
| loki | 128Mi |
| nemo-guardrails | 512Mi |
| ollama | 1Gi |
| opa | 128Mi |
| postgresql | 256Mi |
| presidio | 384Mi |
| prometheus-stack | 752Mi |
| qdrant | 256Mi |
| tempo | 128Mi |
| temporal | 384Mi |
| text-embeddings-inference | 512Mi |
| unstructured | 512Mi |
| valkey | 64Mi |
| **Total** | **~6.6Gi** |

Verdict: Exceeds the comfortable 8Gi headroom when combined with OS, k3d overhead, Cilium (~300Mi), ArgoCD (~500Mi), and CoreDNS. Total system pressure is approximately 8-9Gi — on the edge for a 16Gi host and likely causes OOM kills on 8Gi available RAM.

---

## Executive Summary

The individual apps are mostly well-tuned for homelab use — resource requests are conservative and replicas are consistently set to 1. The critical problem is additive: the `agent-full` profile stacks all 19 apps simultaneously, pushing declared memory requests to ~6.6Gi before accounting for Cilium, ArgoCD, CoreDNS, and system overhead, which together consume another 1-2Gi. On a machine with 8Gi available for k3d this will reliably cause OOM evictions under moderate workload. Three specific issues dominate: (1) Langfuse's bundled ClickHouse and MinIO have no resource constraints at all, making their actual consumption opaque and uncontrolled; (2) Temporal ships with multiple internal services (frontend, matching, history, worker) that the values file does not individually constrain, so its actual footprint exceeds the 384Mi declared in the top-level `server` values; (3) the Prometheus retention PVC at 20Gi and the Ollama model PVC at 20Gi bring total PVC allocations to 79Gi, which may exhaust local-path storage on a developer laptop with a modest SSD. The `agent-minimal` and `rag` profiles are genuinely safe for homelab use; `agent-full` is not without intervention.

---

## Finding 1: Langfuse Bundled Sub-Charts Have No Resource Constraints

**Severity**: Critical
**Domain**: Homelab
**Affected app**: langfuse
**Root cause hypothesis**: The langfuse values file only sets resources for the main application container. Two bundled sub-charts (`clickhouse.deploy: true` and `s3.deploy: true`) inherit their upstream defaults with no homelab overrides. ClickHouse is known to allocate 2-4Gi RAM at startup with default settings; MinIO typically requests 256-512Mi. Neither has limits set, so a single app can silently consume most of the cluster's available memory.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/langfuse.yaml` lines 37-58: `clickhouse.deploy: true` and `s3.deploy: true` with no `resources:` stanza under either sub-chart.
- Langfuse app resources at lines 53-59: only 256Mi request / 1Gi limit — but this covers only the Next.js frontend, not ClickHouse or MinIO.
- ClickHouse upstream Helm chart default: 2Gi memory request with no limit.
**Cross-domain flag**: yes — also a cost/reliability concern if ClickHouse OOMs and corrupts its data directory on the hostPath volume.
**Investigation direction**: Add `clickhouse.resources` and `s3.resources` overrides to `langfuse.yaml`. For ClickHouse, set `memory: 512Mi` request / `1Gi` limit and tune `clickhouse.settings.max_memory_usage` to match. Evaluate whether a standalone ClickHouse catalog entry (per `project_langfuse_deps.md` memory) would give better visibility and control.

---

## Finding 2: Temporal Helm Chart Deploys Multiple Services — Only Top-Level Resources Are Set

**Severity**: High
**Domain**: Homelab
**Affected app**: temporal
**Root cause hypothesis**: The Temporal Helm chart (v0.73.2) deploys four separate server components — frontend, history, matching, and worker — each as its own Deployment. The values file only sets `server.replicaCount: 1` and `server.resources`, which may apply globally or only to a parent chart wrapper. In practice, each Temporal service pod runs independently and the chart also deploys an `admintools` init Job and a `schema` setup Job. Without per-service resource overrides, actual cluster consumption is likely 4× the declared 256Mi request.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/temporal.yaml` lines 4-13: single `server.resources` block with no per-service (frontend/history/matching/worker) breakdown.
- The temporal Helm chart GitHub source (go.temporal.io/helm-charts v0.73.2) defines separate `frontend`, `history`, `matching`, `worker` top-level keys each accepting their own `resources` and `replicaCount`.
**Cross-domain flag**: no
**Investigation direction**: Expand the temporal values file to explicitly set `frontend.resources`, `history.resources`, `matching.resources`, and `worker.resources` each to 128Mi request / 512Mi limit. Also set `admintools.resources` to prevent unbounded init containers. Verify which key `server.resources` actually maps to in the chart.

---

## Finding 3: agent-full Profile Exceeds Safe Memory Envelope for 8-16Gi Hosts

**Severity**: High
**Domain**: Homelab
**Affected app**: profile: agent-full (all 19 apps)
**Root cause hypothesis**: The `agent-full` profile enables every catalog app simultaneously. Declared memory requests total ~6.6Gi. Adding Cilium (~300Mi), ArgoCD controller + server + applicationset-controller (~500Mi), CoreDNS (~50Mi), k3d node agents (~100Mi), and OS baseline yields a system-wide pressure of 7.5-9Gi. On a host with 8Gi available for k3d, the Linux OOM killer will evict pods under any load spike. On a 16Gi host it is marginal.
**Evidence**:
- Profile definition: `/Users/alicanalbayrak/dev/sikifanso/sikifanso/internal/profile/profile.go` lines 29-37: all 19 apps listed.
- Memory request sum computed from values files: ~6.6Gi (see Profile Memory Request Totals table above), not including Langfuse sub-charts.
- Ollama alone requests 1Gi with a 4Gi limit; prometheus-stack requests 752Mi.
**Cross-domain flag**: no
**Investigation direction**: Consider making `agent-full` a documentation-only "max configuration" label and recommending `agent-dev` or `agent-safe` as the practical starting profiles. Alternatively, gate `agent-full` with a preflight check in the CLI (`sikifanso cluster create`) that reads available node memory and warns before enabling this profile.

---

## Finding 4: Prometheus PVC Set to 20Gi — Exact Size of Ollama Model PVC

**Severity**: High
**Domain**: Homelab
**Affected app**: prometheus-stack
**Root cause hypothesis**: Prometheus is configured with `retention: 7d` and a 20Gi storage claim. For a single-node homelab cluster scraping perhaps 20-40 pods, 7-day retention consumes far less than 20Gi (typical: 1-3Gi). The 20Gi figure was likely copied from a production template. Combined with Ollama's 20Gi model PVC, Loki's 10Gi, Tempo's 10Gi, PostgreSQL's 10Gi, Qdrant's 10Gi, AlertManager's 5Gi, Grafana's 2Gi, and Valkey's 2Gi, total PVC allocation reaches 89Gi. Local-path provisioner on a developer laptop with a 256-512GB SSD may not comfortably accommodate this alongside Docker images and OS files.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/prometheus-stack.yaml` line 23-25: `storage: 20Gi`.
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/ollama.yaml` line 25: `size: 20Gi`.
- Combined PVC footprint: alloy(0) + loki(10Gi) + prometheus(20Gi) + alertmanager(5Gi) + grafana(2Gi) + tempo(10Gi) + postgresql(10Gi) + qdrant(10Gi) + ollama(20Gi) + valkey(2Gi) = **89Gi total**.
**Cross-domain flag**: no
**Investigation direction**: Reduce prometheus PVC to 5Gi (sufficient for 7-day homelab retention). Reduce AlertManager PVC to 1Gi (alert state is tiny). Consider reducing Ollama to 10Gi if only small models (llama3.2:1b = ~800MB) are pulled. Total PVC savings available: ~31Gi.

---

## Finding 5: cnpg-operator Has No Resource Requests or Limits

**Severity**: Medium
**Domain**: Homelab
**Affected app**: cnpg-operator
**Root cause hypothesis**: The cnpg-operator values file sets `replicaCount: 1`, `crds.create: true`, and `config.clusterWide: true`, but contains no `resources:` stanza. The CNPG operator is a Go controller-manager and typically uses 50-100Mi at idle, but it has no upper bound set. On a single-node cluster with no resource isolation, an operator bug or a large number of cluster reconciliations could consume unbounded memory.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/cnpg-operator.yaml`: no `resources:` key present.
- All other operator-style apps in the catalog (external-secrets, opa) have explicit resources set.
**Cross-domain flag**: no
**Investigation direction**: Add a `resources:` block with `requests: {memory: 64Mi, cpu: 50m}` and `limits: {memory: 256Mi, cpu: 200m}`. These match the pattern used for external-secrets and opa operators.

---

## Finding 6: Ollama Memory Limit (4Gi) Is Conservative for CPU-Only Inference

**Severity**: Medium
**Domain**: Homelab
**Affected app**: ollama
**Root cause hypothesis**: Ollama is configured for CPU-only mode with `llama3.2:1b` (quantized model, ~800MB on disk). The 4Gi memory limit is generous for a 1B parameter model but appropriate if users load larger models later. The 1Gi request is realistic. The risk is that operators who add larger models (e.g., llama3.1:8b = ~4.7GB) will silently OOM because the limit is set to 4Gi and the model won't fit. There is no documentation in the values file warning that the limit must be raised before pulling larger models.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/ollama.yaml` lines 6-17: `models.pull: [llama3.2:1b]`, `limits.memory: 4Gi`.
- llama3.1:8b Q4 quantization requires ~4.7GB RAM for CPU inference, which exceeds the 4Gi limit.
**Cross-domain flag**: yes — litellm-proxy references `ollama/llama3.2:1b` explicitly; changing the model requires updating both files.
**Investigation direction**: Add a comment in the values file listing the memory requirement for common models (1b = ~1Gi, 3b = ~2.5Gi, 8b = ~5Gi). Consider whether the limit should be raised to 6Gi to safely accommodate 7B/8B models, and update the litellm-proxy values accordingly if the model is changed.

---

## Finding 7: Temporal Values File Does Not Disable Cassandra or Elasticsearch Schema Jobs

**Severity**: Medium
**Domain**: Homelab
**Affected app**: temporal
**Root cause hypothesis**: The Temporal Helm chart's default values include schema setup jobs for multiple persistence backends. The values file correctly disables `cassandra.enabled: false` and `mysql.enabled: false`, and sets `schema.setup.enabled: true`. However, the chart may still render Elasticsearch/OpenSearch schema jobs unless explicitly disabled. Temporal v1.x charts default to running additional schema migration jobs that can fail on startup if Elasticsearch is not present, causing CrashLoopBackOff noise.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/temporal.yaml` lines 29-32: only `cassandra`, `postgresql`, and `mysql` are explicitly disabled. No `elasticsearch.enabled: false` present.
- The temporal Helm chart (v0.73.2) includes an `elasticsearch` sub-chart enabled by default in some versions.
**Cross-domain flag**: no
**Investigation direction**: Add `elasticsearch.enabled: false` to the temporal values file. Also verify `schema.update.enabled: false` is set if schema migrations are handled by `schema.setup` only. Review the chart's `values.yaml` to confirm which sub-charts are enabled by default in v0.73.2.

---

## Finding 8: Prometheus Retention at 7 Days Is Fine, But No Retention Size Guard

**Severity**: Low
**Domain**: Homelab
**Affected app**: prometheus-stack
**Root cause hypothesis**: The values file sets `retention: 7d` but does not set `retentionSize`. Without a size-based retention guard, Prometheus will use the full 20Gi PVC regardless of time-based retention — if ingest rate is high (e.g., many scrape targets at 15s interval), the TSDB can grow to fill the 20Gi before the 7-day window expires, causing silent data loss via head compaction rather than a clear error.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/prometheus-stack.yaml` lines 18-20: `retention: 7d` with no `retentionSize` key.
**Cross-domain flag**: no
**Investigation direction**: Add `retentionSize: "4GB"` under `prometheusSpec` (assumes 5Gi PVC after reducing per Finding 4). This ensures the TSDB never fills the volume. Also consider reducing the scrape interval from the default 1m to 2m for homelab use to reduce ingest pressure.

---

## Finding 9: Agent Sandbox ResourceQuota Sets requests.limit Equal to limits.limit

**Severity**: Low
**Domain**: Homelab
**Affected app**: agent sandbox template
**Root cause hypothesis**: The ResourceQuota template hard-sets `requests.cpu`, `limits.cpu`, `requests.memory`, and `limits.memory` all to the same value (agent.cpu and agent.memory respectively). This means every pod in the agent namespace must specify both requests and limits, and those values must match the quota cap exactly. This prevents pods from using Burstable QoS class (request < limit), forcing them into either Guaranteed (request == limit) or causing scheduling failures if they don't set limits. For experimental agent workloads this is unnecessarily rigid.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-agent-template/templates/resourcequota.yaml` lines 9-14: `limits.cpu` and `requests.cpu` both set to `{{ .Values.agent.cpu }}`.
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-agent-template/values.yaml` lines 4-8: `cpu: "500m"`, `memory: "512Mi"` — single value used for both requests and limits quota.
**Cross-domain flag**: no
**Investigation direction**: Separate quota values into `agent.cpuRequest` / `agent.cpuLimit` and `agent.memoryRequest` / `agent.memoryLimit`, or set only `limits.cpu` and `limits.memory` in the quota (omitting request-side quota) to allow Burstable pods. This gives agent workloads more flexibility without removing the safety ceiling.

---

## Finding 10: NeMo Guardrails 512Mi Request Is Likely Underestimated

**Severity**: Medium
**Domain**: Homelab
**Affected app**: nemo-guardrails
**Root cause hypothesis**: NeMo Guardrails (NVIDIA) is a Python-based LLM guardrail framework that loads NLP models and a LangChain/LlamaIndex runtime at startup. The 512Mi memory request is likely insufficient for the default configuration, which loads SpaCy models and potentially downloads additional NLTK/transformers data on first run. The 1Gi limit may also be too low if the application loads any local embedding models for semantic similarity checks.
**Evidence**:
- `/Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap/catalog/values/nemo-guardrails.yaml` lines 7-11: `requests.memory: 512Mi`, `limits.memory: 1Gi`.
- NeMo Guardrails documentation notes that the ColangRuntime and LangChain integration layers each require 200-400MB base memory before loading any rails configuration.
**Cross-domain flag**: yes — nemo-guardrails is paired with guardrails-ai (256Mi request) in both `agent-safe` and `agent-full`; both running simultaneously adds 768Mi+ in requests for guardrail-layer apps alone.
**Investigation direction**: Run `nemo-guardrails` in isolation and observe actual RSS before setting values. If using the default colang runtime without local LLMs, 768Mi request / 1.5Gi limit is a safer estimate. If semantic similarity checks via a local embedding model are enabled, increase to 1Gi request / 2Gi limit.

---

## Summary of PVC Totals by Profile

| Profile | Total PVC Allocated |
|---------|-------------------|
| agent-minimal | 10Gi (postgresql only) |
| agent-dev | 52Gi (postgresql 10 + ollama 20 + qdrant 10 + loki 10 + valkey 2) |
| agent-safe | 42Gi (postgresql 10 + ollama 20 + qdrant 10 + valkey 2) |
| rag | 30Gi (postgresql 10 + qdrant 10 + tempo 10) |
| agent-full | 89Gi (all apps) |

**Recommendation**: The 89Gi total for `agent-full` should be documented prominently. Users on laptops with limited SSD space should be warned before enabling the full profile.
