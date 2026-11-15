# ADR-0002: Catalog ordering via app-of-apps and sync waves

**Status:** Accepted
**Date:** 2026-08-10
**Arose from:** sikifanso/sikifanso#56
**Follows:** [ADR-0001](0001-git-is-the-source-of-desired-state.md)

## Context

The requirement is that cross-cutting applications deploy first and tear down last: operators
before the data layer before services, reversed on removal. ADR-0001 requires that ordering be
declared in the repo rather than applied by the CLI.

Two mechanisms currently exist and have coexisted since April 2026:

- `bootstrap/root-catalog.yaml` in `sikifanso-homelab-bootstrap` sets
  `strategy.type: RollingSync` with three steps keyed on a `tier` label (commit `43f9e29`,
  2026-04-04), and `internal/infraconfig/defaults/argocd-values.yaml` passes
  `--enable-progressive-syncs`.
- `watchApps` applies its own tier batching, added the *following day* (`9a6c72e`, 2026-04-05).

Neither has shipped: both repos last tagged `v0.9.1` on 2026-03-31, and tagged CLI releases
clone the bootstrap repo at the matching tag (`resolveBootstrapVersion`). RollingSync has only
ever run on dev/HEAD builds, so removing it costs no released user anything.

All findings below were verified against **ArgoCD v3.4.5**, the `appVersion` of the pinned
`argo-cd` chart 10.2.1.

1. [verified] **RollingSync strips automated sync.**
   `applicationset/controllers/applicationset_controller.go:1613-1617` — *"ensure that
   Applications generated with RollingSync do not have an automated sync policy, since the
   AppSet controller will handle triggering the sync operation instead"* — sets
   `Automated.Enabled = new(false)` on every generated Application. The catalog AppSet template
   asks for `selfHeal: true`; what lands in the cluster has automated sync off. `prune` is
   preserved and carried into the controller-triggered sync; `selfHeal` is not. Catalog
   applications therefore do not self-heal drift, and the comment in the manifest asserting
   selfHeal as a race guard describes behaviour that does not exist. Behaviour dates to 2022
   (upstream #9437), so it is not version-fragile.

2. [verified] **`deletionOrder: Reverse` does not cover `app disable`.**
   `applicationset_controller.go:154` gates `performReverseDeletion` (line 171) on
   `DeletionTimestamp != nil` — the ApplicationSet CR itself being deleted. Disabling an app
   drops it from the generator's results, which is handled by `deleteInCluster`: an `errgroup`
   with a concurrency limit and no ordering. RollingSync cannot express the teardown half of
   the requirement.

3. [verified] **Sync waves give both directions from one annotation.**
   `gitops-engine/pkg/sync/sync_tasks.go:85-110` sorts tasks by phase, then ascending wave.
   `gitops-engine/pkg/sync/sync_context.go:1075-1097` — *"for prune tasks, modify the waves for
   proper cleanup i.e reverse of sync wave (creation order)"* — reassigns prune waves by
   symmetric swap. ArgoCD deploys ascending and prunes descending, natively.

4. [verified] **App-of-apps does not gate on child health by default.**
   `gitops-engine/pkg/health/health_test.go:112` asserts `getHealthStatus` returns `nil` for
   both healthy and degraded `Application` fixtures — there is no built-in health check for
   `argoproj.io/Application`. `controller/health.go:80-83` additionally skips child apps in
   `Missing`/`Unknown` so they do not affect the parent. Without a health customisation, waves
   advance immediately and gate on nothing.

5. [verified] **The untiered-app hazard is latent but real.** `DefaultTier` is `0-operators`
   (`internal/argocd/grpcsync/types.go:48`), so an entry with no tier is treated client-side as
   an *operator* — the most privileged position, deployed first and removed last. RollingSync
   disagrees in the opposite extreme: an `In` matcher skips an app with no `tier` label, so it
   is never marked Pending, never synced by the controller, and has automated sync stripped —
   it would sit unsynced with no error. All 19 catalog entries currently carry one of the three
   known tiers, so neither behaviour is live today.

## Decision

**Move the catalog track from ApplicationSet + RollingSync to app-of-apps + sync waves.**

- The CLI renders one `Application` manifest per enabled catalog entry into the gitops repo,
  carrying `argocd.argoproj.io/sync-wave` derived from the entry's `tier`.
- A single parent `Application` points at that directory; ArgoCD creates children in ascending
  wave order and prunes them in descending order.
- Children keep `syncPolicy.automated` with `selfHeal` and `prune`, because no RollingSync
  rewrites them.
- `strategy.rollingSync` and `--enable-progressive-syncs` are removed.
- `watchApps` sheds its tier batching entirely, along with `waitForAppear`'s manual sync, which
  would otherwise pre-empt the wave sequence.
- A `resource.customizations.health.argoproj.io_Application` entry is added so waves gate on
  child application health — the same mechanism `argocd-values.yaml` already uses for Ingress.
- Catalog entries must be validated to carry a known tier, so finding 5 cannot become live.

Both ordering directions end up declared in the repo, and the CLI is out of the ordering
business completely — it writes files, nudges, and watches.

## Alternatives considered

**A — keep both mechanisms, document the interaction.** Leaves forward ordering duplicated in
two repos, leaves `waitForAppear` pre-empting the step gating, and still loses drift correction
to finding 1. Strictly worse than C.

**B — drop RollingSync, make the CLI the sole sequencer.** Simplest change and the one with the
smallest diff, but it moves the ordering policy *out* of the repo and into a Go binary, which
is exactly what ADR-0001 forbids. Rejected on that ground, not on cost.

**C — keep RollingSync for forward ordering, keep the CLI's reversed teardown.** Each mechanism
would do the half the other cannot, with no duplication, and forward ordering would be declared.
Rejected because teardown ordering stays undeclared (finding 2) and drift correction stays lost
(finding 1) — D gets both back for the price of a restructure.

## Consequences

- **Cross-repo change.** `bootstrap/root-catalog.yaml` becomes a plain `Application` instead of
  an ApplicationSet. The triple-track model in `CLAUDE.md` needs rewriting: the catalog track is
  no longer AppSet-generated.
- **The CLI becomes a template renderer**, and the rendered `Application` manifests are
  committed. This is the *rendered manifests* pattern rather than the generator pattern — a
  deliberate trade, and one that makes what ArgoCD sees directly inspectable in the repo.
- **Drift correction returns** for catalog applications.
- **Teardown ordering becomes a cluster property**, so it holds for a `kubectl delete` of a
  rendered manifest, not only for `sikifanso app disable`.
- **One behaviour needs verifying on a real cluster** and cannot be checked in an agent session:
  whether the parent waits for a wave's child deletions to *complete* — children carry
  `resources-finalizer.argocd.argoproj.io`, so deletion blocks on their own resources — before
  advancing to the next prune wave. If it does not, teardown ordering is nominal only.
- **Supersedes the arrangement in bootstrap commit `43f9e29` and CLI PR #13.** Since neither
  shipped, no released user is affected and no migration is required.
