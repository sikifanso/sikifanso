# ArgoCD Deployment Research — Progress Tracker

**Source report**: [2026-04-04-argocd-deployment-research-report.md](2026-04-04-argocd-deployment-research-report.md)
**Started**: 2026-04-04
**Last updated**: 2026-04-05

---

## Problem Status

| ID | Severity | Title | Status | Commit / Notes |
|----|----------|-------|--------|----------------|
| P1 | Critical | watchSingleApp exits on transient Degraded | **Done** | `6f30eb2` — Degraded grace period |
| P2 | Critical | Cluster create post-profile sync uses wrong op | **Done** | `bfe0804` — use OpEnable; `46bc029` — include auto-added deps |
| P3 | Critical | Catalog ApplicationSet has no automated SyncPolicy | **Done** | Part of `62cf234` — selfHeal + prune added to root-catalog.yaml |
| P4 | High | ArgoCD webhook/gRPC not ready after install | **Done** | gRPC Version probe between waitForDeployments and CreateApplications |
| P5 | High | Hardcoded ~/.kube/config in Install/CreateApplications | **Done** | Thread *rest.Config via kube.RESTConfigForCluster |
| P6 | High | RollingSync tier ordering ignored by CLI + missing tiers | **Done** | `62cf234` — RollingSync + dependency graph |
| P7 | High | agent-minimal missing valkey | **Done** | `43edb58` — add valkey to agent-minimal |
| P8 | High | Langfuse ClickHouse/MinIO no resource constraints | **Done** | resourcesPreset already set; added ClickHouse max_memory_usage cap |
| P9 | High | Presidio analyzer OOM (512Mi limit, needs ~800Mi) | **Done** | PR #25 — raise to 768Mi/1Gi for spaCy en_core_web_lg |
| P10 | Medium | pollOnce fallback gives single stale snapshot | **Done** | `fc64572` — pollUntilTerminal loop; `1417023`, `4fd8bf1` — review fixes |
| P11 | Medium | waitForAppear always waits >=5s (no initial check) | Open | |
| P12 | Medium | SyncApplication called without operation-state guard | Open | |
| P13 | Medium | Error messages discard all context | Open | |
| P14 | Medium | Default branch conflates Progressing and Unknown | Open | |
| P15 | Medium | Spinner suffix data race under concurrent watches | Open | |
| P16 | Medium | ReconcileFn error swallowed, causes full-timeout block | Open | |
| P17 | Medium | Root ApplicationSets start concurrently — resource contention | Open | |
| P18 | Medium | ResourceTree misses sync error messages | Open | |
| P19 | Medium | gRPC connection has no keep-alive | Open | |
| P20 | Medium | agent-full exceeds safe memory for 8-16 GiB hosts | Open | |
| P21 | Medium | Temporal values missing per-service resources + Elasticsearch | Open | |
| P22 | Medium | Prometheus PVC oversized at 20 GiB | Open | |
| P23 | Medium | Agent sandbox ResourceQuota forces Guaranteed QoS | Open | |
| P24 | Low | cnpg-operator has no resource requests/limits | Open | |
| P25 | Low | nemo-guardrails has no LLM endpoint configured | Open | |

---

## Task Status

| ID | Priority | Title | Status | Related |
|----|----------|-------|--------|---------|
| T1 | Critical | Degraded grace period in watchSingleApp | **Done** | P1 |
| T2 | Critical | Fix cluster create post-profile sync (OpEnable) | **Done** | P2 |
| T3 | Critical | Resource constraints for Langfuse ClickHouse/MinIO | **Done** | P8 — presets already in place; added engine-level memory cap |
| T4 | Critical | Automated SyncPolicy for catalog ApplicationSet | **Done** | P3 |
| T5 | High | Fix hardcoded ~/.kube/config | **Done** | P5 — use kube.RESTConfigForCluster, thread restCfg through all callees |
| T6 | High | Add missing tier/dependsOn to 14 catalog entries | **Done** | P6 — verified: all 19 entries have tiers, all dependency chains complete |
| T7 | High | Add valkey to agent-minimal + langfuse dependsOn | **Done** | P7 |
| T8 | High | Fix presidio analyzer memory limit | **Done** | P9 — PR #25 |
| T9 | Medium | Replace pollOnce with retry loop | **Done** | P10 |
| T10 | Medium | Tier-aware watchApps goroutine sequencing | **Done** | P6 — PR #13; tier-aware sequencing + reverse order for disable |
| T11 | Medium | ArgoCD gRPC readiness probe after install | **Done** | P4 — WaitForGRPC polls Version endpoint; installInfra extracted from Create |
| T12 | Medium | Startup ApplicationSet sequencing in cluster create | Open | P17 |
| T13 | Medium | Actionable error messages from Result slice | Open | P13 |
| T14 | Medium | Fix spinner data race + multi-app progress | Open | P15 |
| T15 | Medium | Temporal per-service resources + disable Elasticsearch | Open | P21 |
| T16 | Medium | Reduce Prometheus PVC + add retentionSize guard | Open | P22 |
| T17 | Medium | Separate ResourceQuota requests/limits in agent-template | Open | P23 |

---

## Summary

- **Completed**: 14/17 tasks (T1–T11 + associated review fixes)
- **Remaining Critical**: 0
- **Remaining High**: 0
- **Remaining Medium**: 6 (T12–T17)
- **Low (no task)**: 2 (P24, P25)
