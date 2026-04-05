# ArgoCD Deployment Research Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to execute this plan.

**Goal:** Run five parallel specialist agents to identify reliability/UX problems in the sikifanso ArgoCD deployment pipeline and resource-sizing/shared-services problems across all homelab catalog apps, then synthesize findings into a prioritized problem report.

**Architecture:** Five domain-expert agents run in parallel and write structured findings docs to `docs/superpowers/research/`. A synthesis agent reads all five docs and writes the final report to `docs/superpowers/reports/`.

**Tech Stack:** Go (urfave/cli/v3, ArgoCD gRPC SDK, client-go), ArgoCD, k3d/k3s, Helm, Kubernetes

---

## Status: COMPLETED 2026-04-04

All five agents ran and produced findings. Synthesis complete.

**Output**:
- `docs/superpowers/research/findings-argocd.md` — 9 findings (2 Critical, 3 High)
- `docs/superpowers/research/findings-k8s.md` — 8 findings (1 Critical, 3 High)
- `docs/superpowers/research/findings-go.md` — 9 findings (1 Critical, 3 High)
- `docs/superpowers/research/findings-homelab.md` — 10 findings (1 Critical, 3 High)
- `docs/superpowers/research/findings-appworkloads.md` — 19 findings (3 High)
- `docs/superpowers/reports/2026-04-04-argocd-deployment-research-report.md` — 25 deduplicated problems, 4 cross-domain insights, 17 investigation tasks

See the report for full details and investigation task list.
