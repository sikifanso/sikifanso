# ADR-0001: Git is the source of desired state; the API observes and nudges

**Status:** Accepted
**Date:** 2026-08-10
**Arose from:** sikifanso/sikifanso#56

## Context

This project exists to run a cluster shaped the way a production platform would be shaped:
Cilium for networking, ArgoCD for delivery, applications declared in a gitops repo that
ArgoCD's repo-server reads directly from `/local-gitops`. Three ApplicationSets generate
`Application` CRs from files the CLI writes.

The CLI also talks to ArgoCD over gRPC. Some of that is unavoidable, some is a latency
optimisation, and some of it — discovered while triaging #56 — turned out to be *desired
state expressed in Go instead of in the repo*.

The concrete instance: catalog applications are deployed in tier order (`0-operators` before
`1-data` before `2-services`), and torn down in reverse. That ordering is applied in
`internal/argocd/grpcsync/orchestrator.go` (`watchApps`), reached from exactly two callers —
`cmd/sikifanso/cluster_create.go` and `syncAfterMutation` in `cmd/sikifanso/middleware.go`.
`cluster.Start` applies none of it.

The consequence is testable: clone the gitops repo, point a bare ArgoCD at it, and the
ordering is gone. It was never declared anywhere. Anything that does not go through a
`sikifanso` mutation command — a hand edit to `catalog/*.yaml`, the 180s reconciliation
fallback, a sync from the ArgoCD UI — gets no ordering at all.

Without a stated boundary, each new command re-decides this question, and the answer drifts
toward whatever is easiest to implement in the client.

## Decision

**Git is the only source of desired state. The API may be used to observe, and to ask a
controller to look sooner. It must never decide what is deployed, or in what order.**

"No API calls from the CLI" would be the wrong rule — watching an application's status is an
API call, and so is a webhook. The line is not *whether* the CLI calls the API, but whether
the call carries a decision that should have been a file in the repo.

Classification of current behaviour under the rule:

| Behaviour | Verdict |
|---|---|
| `catalog.SetEnabled` + `gitops.Commit` | ✅ declarative write — this is the sanctioned path |
| `appsetreconcile` annotation patch | ✅ "look sooner"; a webhook by another name, carries no state |
| `grpcsync` watching for progress/health | ✅ observation only |
| `cluster create` creating the `cilium`/`argocd` Application CRs | ✅ bootstrap exception, see below |
| `SyncApplication` in `waitForAppear` | ⚠️ forces convergence toward state already in git — permitted as a nudge, but only where it cannot pre-empt an ordering the cluster is enforcing |
| **tier sequencing in `watchApps`** | ❌ **decides order; exists nowhere in git** |

### Bootstrap exception

Cilium and ArgoCD are installed with the Helm SDK, and `cluster create` imperatively creates
the `cilium` and `argocd` `Application` CRs before handing them to ArgoCD to manage. This is
a genuine chicken-and-egg — ArgoCD cannot manage its own installation before it exists — and
is what Flux and argocd-autopilot do too. The infrastructure track is outside the scope of
this rule; the rule governs the application tracks (`root`, `catalog`, `agents`).

## Consequences

- **Tier ordering must move into the repo.** How is recorded separately in
  [ADR-0002](0002-catalog-ordering-via-sync-waves.md).
- **`waitForAppear`'s manual sync becomes conditional.** It is acceptable as a nudge only
  where the cluster is not itself sequencing. Under an ordering the cluster enforces, a manual
  sync can pre-empt the sequence, and must be removed rather than kept "just in case".
- **`argocd_rollback` (MCP) needs auditing against this rule.** Rolling an application back to
  a previous revision through the API changes desired state without changing git, which would
  leave the repo and the cluster disagreeing. Not assessed yet; it is not covered by the
  bootstrap exception.
- **New commands have a test to apply, not a judgement to make:** *if the CLI vanished and
  someone pointed a bare ArgoCD at the gitops repo, what would they lose?* Anything on that
  list that is not observation, latency, or bootstrap is a bug.
- **Vocabulary gap.** This ADR leans on terms — *declared state*, *nudge*, *tier*, *wave* —
  that `CONTEXT.md` does not define. Per `docs/agents/domain.md` that is a signal to run
  `/domain-modeling` over this area rather than to invent definitions here.
