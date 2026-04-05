# ArgoCD Deployment Research Design
**Date**: 2026-04-04
**Status**: Approved

## Goal

Produce a structured problem report that identifies reliability and UX issues in the
sikifanso ArgoCD deployment pipeline (CLI machinery) and resource-sizing issues across
all homelab catalog apps (bootstrap repo). The report feeds follow-up investigation
sessions that propose targeted fixes.

## Scope

Two parallel tracks:

1. **CLI/ArgoCD machinery** — sync orchestrator, health interpretation, progress
   reporting, error messaging, gRPC watch lifecycle
2. **Homelab catalog apps** — Helm values for all catalog entries, resource sizing for
   a single-node k3d cluster, shared-services audit (apps that bundle infra they
   could borrow from the lab)

## Methodology: Parallel Domain-Expert Panel → Synthesis

Five specialist agents run in parallel. Each produces a structured findings doc. A
synthesis agent then reads all five, deduplicates, resolves cross-domain findings, and
writes the final report.

### Agent 1 — ArgoCD Specialist

**Investigates**:
- `sikifanso/internal/argocd/grpcsync/`
- `sikifanso/internal/argocd/grpcclient/`
- `sikifanso/internal/argocd/appsetreconcile/`

**Key questions**:
- Is `Synced+Degraded` the right exit condition, or does ArgoCD's health model have
  transient Degraded states that don't mean failure?
- Is the gRPC watch stream fallback to a single `pollOnce` snapshot adequate when the
  stream drops mid-sync?
- Is triggering AppSet reconciliation via annotation patch reliable? Are there race
  conditions between the annotation being cleared and the CLI starting `waitForAppear`?
- Does `selfHeal: false` on the Application CRD cause unexpected sync behaviour?
- Is `waitForAppear` polling at 5s intervals the right mechanism?

### Agent 2 — Kubernetes Resource & Health Specialist

**Investigates**:
- `sikifanso/internal/argocd/install.go`
- `sikifanso/internal/cluster/cluster.go`
- The `app enable postgresql` log output

**Key questions**:
- CRD `Degraded` during startup — real failure or startup ordering noise?
- Does `waitForDeployments` check the right condition before `CreateApplications`?
- Single-node k3d: scheduling and resource-contention effects of parallel startup?
- Are there CRD → operator → CR ordering requirements the code doesn't enforce?

### Agent 3 — Go Code Reviewer

**Investigates**:
- `sikifanso/cmd/sikifanso/cluster_create.go`
- `sikifanso/cmd/sikifanso/app_cmd.go`
- `sikifanso/cmd/sikifanso/middleware.go`
- `sikifanso/internal/argocd/grpcsync/orchestrator.go`

**Key questions**:
- Are error messages actionable?
- Is the post-profile sync using the right operation type?
- Is the multi-app spinner progress display correct?
- Is the gRPC client Close() lifecycle correct?
- Is there missing distinction between transient and permanent errors?

### Agent 4 — Homelab / Resource-Constraints Expert

**Investigates**:
- `sikifanso-homelab-bootstrap/catalog/*.yaml`
- `sikifanso-homelab-bootstrap/catalog/values/*.yaml`

**Key questions**:
- Which apps have production-sized defaults that exceed homelab capacity?
- What is the total resource envelope per profile on 8–16 GB available RAM?
- Which apps have documented minimum-viable Helm values?
- Are there apps that fundamentally cannot fit a homelab node?

### Agent 5 — K8s App Workloads Specialist

**Investigates**:
- All catalog entries in `sikifanso-homelab-bootstrap/catalog/`
- `sikifanso-agent-template/`

**Key questions**:
- **Shared-services audit**: which apps bundle PostgreSQL, Redis, MinIO/S3, ClickHouse,
  or other infra? For each: is the lab's shared instance a viable replacement?
- Are there apps where the shared service cannot be a safe drop-in?
- What is the total resource envelope per profile when all co-deployed apps run?
- Are agent sandbox ResourceQuota values appropriate for AI workloads?

### Synthesis Agent

Reads all five findings docs and:
1. Deduplicates findings that share a root cause
2. Resolves cross-domain flags — writing combined insights
3. Ranks by user impact
4. Generates investigation tasks

## Finding Output Format

```
## Finding [N]: [Short Title]
**Severity**: Critical | High | Medium | Low
**Domain**: ArgoCD | K8s | Go | Homelab | AppWorkloads
**Affected scenario**: e.g. "app enable with auto-deps"
**Root cause hypothesis**: what we believe is causing this
**Evidence**: specific file:line references, config excerpts, log snippets
**Cross-domain flag**: yes/no
**Investigation direction**: what a follow-up session should examine
```

## Final Report Location

`docs/superpowers/reports/2026-04-04-argocd-deployment-research-report.md`

## Permissions

Read/Glob/Grep auto-approved for:
- `/Users/alicanalbayrak/dev/sikifanso/**`
- `/Users/alicanalbayrak/dev/sikifanso-homelab-bootstrap/**`
- `/Users/alicanalbayrak/dev/sikifanso-agent-template/**`

Configured in `.claude/settings.local.json`.

## Known Context

The `app enable postgresql` execution produced this observed failure:
- `prometheus-stack` reported `sync=Synced health=Degraded` while resources were still
  `Progressing` (PVCs, Pods, CRDs)
- One image pull failed with EOF — transient network error
- The CLI exited with "sync failed: one or more apps unhealthy" even though the app
  recovered later
- All auto-enabled dependencies deployed simultaneously with no ordering

The Langfuse deployment failed due to ClickHouse requesting ~12 GB on a single homelab
node — a resource-sizing issue in the bootstrap repo values.
