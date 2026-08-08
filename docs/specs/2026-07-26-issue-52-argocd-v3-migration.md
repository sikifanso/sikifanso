# Migrate the argo-cd Go module v2 → v3

**Date:** 2026-07-26
**Status:** Implemented
**Issue:** sikifanso/sikifanso#52
**Branch:** `design/issue-52-argocd-v3`
**Approved:** 2026-07-26, by the `/implement-design 52` invocation that carried out this work.

## Problem

The deployed ArgoCD server and the Go client library have drifted a full major apart.
`internal/infraconfig/defaults/argocd.yaml:4` pins chart `argo-cd` **10.2.1**, which
deploys ArgoCD **v3.4.5**; `go.mod:7` still requires
`github.com/argoproj/argo-cd/v2 v2.14.20`, the module behind
`internal/argocd/grpcclient/` and `WaitForGRPC` (`internal/argocd/install.go:153`).
`renovate.json:41-45` deliberately disables this major, so it can only arrive as a
hand-written change — and per that config's own comments, `k8s.io/*` and
`helm.sh/helm/v3` are version-locked to whatever argo-cd requires, which is why the
open Renovate PRs #28 (kubernetes group) and #48 (helm v4) are stuck behind it.

This spec was written after performing the entire migration on a throwaway copy of the
repo and running the build, the race-enabled test suite, and golangci-lint against it,
plus a reverse-probe of PR #28's target versions. Everything below labeled [verified]
was directly observed, not read off a changelog.

One correction to the issue text: `internal/argocd/grpcsync/` does **not** import the
argo-cd module. It consumes only `grpcclient`'s domain types (`AppStatus`, `WatchEvent`,
…) — verified by inspecting its imports. The migration therefore touches exactly six
files, all in `internal/argocd/`.

## Findings

1. [verified] The v2 import sites are exactly six files:
   `internal/argocd/install.go:11`, `internal/argocd/install_test.go:9`,
   `internal/argocd/grpcclient/client.go:11-15`,
   `internal/argocd/grpcclient/applications.go:7-8`,
   `internal/argocd/grpcclient/applicationsets.go:7-8`,
   `internal/argocd/grpcclient/projects.go:7-8`.
   This matches the "6 files" count in the renovate.json comment.

2. [verified] Chart `argo-cd` 10.2.1 has `appVersion: v3.4.5`
   (<https://github.com/argoproj/argo-helm/blob/argo-cd-10.2.1/charts/argo-cd/Chart.yaml>).
   v3.4.5 is also the newest v3 tag upstream at authoring time, so "match the deployed
   server minor" and "latest" coincide: target **`github.com/argoproj/argo-cd/v3 v3.4.5`**.

3. [verified] argo-cd v3.4.5's `go.mod` (tag `v3.4.5`, upstream repo): module path
   `github.com/argoproj/argo-cd/v3`, **`go 1.26.0`**, `k8s.io/* v0.34.0`,
   `sigs.k8s.io/structured-merge-diff/v6 v6.3.2`, `github.com/go-git/go-git/v5 v5.14.0`
   (annotated upstream "DO NOT BUMP UNTIL go-git/go-git#1551 is fixed"),
   `github.com/cyphar/filepath-securejoin v0.6.1`.

4. [verified] **The issue's "expected to be small" hides one real landmine.** Since
   v3.3, argo-cd vendors gitops-engine as an *untagged nested module*
   (`gitops-engine/` subdirectory) and pins it via
   `replace github.com/argoproj/argo-cd/gitops-engine => ./gitops-engine`. Replace
   directives do not apply transitively, so consumers must satisfy the bare require —
   `github.com/argoproj/argo-cd/gitops-engine v0.7.1-0.20250908182407-97ad5b59a627` —
   and that version **does not exist**:

   ```
   $ curl https://proxy.golang.org/github.com/argoproj/argo-cd/gitops-engine/@v/v0.7.1-0.20250908182407-97ad5b59a627.info
   not found: ... invalid version: missing github.com/argoproj/argo-cd/gitops-engine/go.mod at revision 97ad5b59a627
   ```

   (The version string was carried over from the old `github.com/argoproj/gitops-engine`
   module when the path was renamed; at that commit the nested `go.mod` did not exist.)
   A naive `go get github.com/argoproj/argo-cd/v3@v3.4.5` therefore fails.

5. [verified] The only working fix is a consumer-side `replace`. A direct `require` of a
   resolvable version cannot work: the nested module has no tags, so resolvable versions
   are `v0.0.0-<date>-<hash>` pseudo-versions, and MVS prefers the broken
   `v0.7.1-0.…` (0.7.1 pre-release > 0.0.0 pre-release). The correct pin is the nested
   module **at the v3.4.5 tag commit** (`564b94973b28…`), which the proxy resolves:

   ```
   $ curl https://proxy.golang.org/github.com/argoproj/argo-cd/gitops-engine/@v/564b94973b28.info
   {"Version":"v0.0.0-20260709160802-564b94973b28", ... "Subdir":"gitops-engine","Ref/Hash": v3.4.5 commit}
   ```

6. [verified] **Full dry-run passes.** On a copy of the repo: rewrite the six files'
   imports `/v2` → `/v3`, apply the go.mod changes from the Design section, run
   `go mod tidy` → `go build ./...` succeeds and `go test -race -count=1 ./...` passes
   for all 17 packages **with zero further source changes**. The gRPC API surface this
   repo uses (apiclient `NewClient`/`ClientOptions`/sub-client constructors; session,
   application, applicationset, project, version services; the `v1alpha1` types) is
   unchanged between v2.14 and v3.4.

7. [verified] `golangci-lint run ./...` on the migrated copy fails with exactly two
   staticcheck `SA1019` deprecations — the only genuine API deltas:
   - `internal/argocd/grpcclient/applications.go:67` — `app.Status.Health.Message` is
     deprecated ("not used and will be removed in a future release").
   - `internal/argocd/grpcclient/applications.go:308` — `RunResourceAction` is
     deprecated in favor of `RunResourceActionV2`.

8. [verified] Dropping the `Health.Message` read is behavior-neutral against the server
   we deploy: v3.4.5's `controller/health.go` (`setApplicationHealth`) computes only a
   `HealthStatusCode` for app-level health and never sets a message (messages exist only
   per-resource, which `ResourceStatus.Message` still carries). Every consumer of the
   app-level `Message` (`cmd/sikifanso/app_status.go:67`, `internal/mcp/argocd.go:74`,
   the `grpcsync` progress plumbing) already guards on the empty string.

9. [verified] `ResourceActionRunRequestV2` has the same fields as V1 (`name`,
   `namespace`, `resourceName`, `version`, `group`, `kind`, `action`) plus optional
   action parameters we don't use (`server/application/application.proto` at v3.4.5,
   messages at line 214/227, rpcs at 511/520). Blast radius of the swap is zero:
   `Client.RunResourceAction` currently has no callers outside `grpcclient` itself.

10. [verified] Helm↔k8s pairing (each helm minor's `go.mod` on the module proxy):
    3.16→k8s 0.31, 3.18→0.33, **3.19→0.34**, 3.20→0.35, 3.21.3→0.36.2. The k8s-0.34
    partner for argo-cd v3.4 is **helm v3.19.x** (latest patch v3.19.5). After the
    migration `go mod tidy` selects `k8s.io/* v0.34.2` (pulled by helm 3.19.5's patch
    requirement — argo-cd's own floor is 0.34.0).

11. [verified] **PR #28's targets do not build on the migrated tree** — the acceptance
    criterion "kubernetes-group PRs become resolvable" cannot mean "at latest".
    Reverse-probe: `go get k8s.io/{api,apimachinery,client-go}@v0.36.3
    helm.sh/helm/v3@v3.21.3` on the migrated copy fails to build because k8s.io/api
    0.36 removed packages (`autoscaling/v2beta1`, `autoscaling/v2beta2`,
    `scheduling/v1alpha1`) that `k8s.io/kubernetes` v1.34.2 internals — required by
    argo-cd v3.4.5 — still import (0.36 also splits out a new `k8s.io/streaming`
    module). The kubernetes group is structurally capped at **k8s < 0.35 / helm < 3.20**
    until argo-cd itself moves its k8s minor.

12. [verified] The migration dissolves the held go-git conflict as a side effect: MVS
    now selects `go-git v5.14.0`, `go-billy v5.6.2`, `filepath-securejoin v0.6.1` —
    exactly the combination the `renovate.json:53` hold comment says argo-cd v2.14
    could not tolerate. Upstream still caps go-git at 5.14 (finding 3), so the hold
    stays, but its description is now wrong.

13. [verified] After the migration the old-path `github.com/argoproj/gitops-engine`
    leaves the module graph entirely (`go list -m` reports "not a known dependency");
    `github.com/argoproj/pkg` remains. The `renovate.json:80-87` "argoproj
    pseudo-versions" group still lists the departed module.

14. [verified] `application.resourceTrackingMethod: annotation`
    (`internal/infraconfig/defaults/argocd-values.yaml`, `configs.cm`) is the server
    default since v3.0 — "Use Annotation-Based Tracking by Default",
    <https://argo-cd.readthedocs.io/en/stable/operator-manual/upgrading/2.14-3.0/#use-annotation-based-tracking-by-default>
    (verified in the v3.4.5 tree's copy of that doc). The line can be dropped.

15. [verified] Go toolchain: the module's `go` directive must rise to `1.26.0`
    (finding 3). Local toolchain is go1.26.5; both CI workflows use
    `go-version-file: go.mod` (`.github/workflows/ci.yml:16`, `release.yml:22`), so
    they follow automatically. `CLAUDE.md:22` ("Go 1.25.") and the k3d hold comment
    (`renovate.json:63`, "module targets 1.25") become stale.

16. [inferred] Renovate's gomod manager may propose updates to the new `replace`
    pseudo-version (pseudo-version bumps are digest-type updates, and the non-major
    batch includes digests). Whether it actually fires on a replace target was not
    tested; a defensive disable rule for `github.com/argoproj/argo-cd/gitops-engine`
    costs three lines and removes the risk of silent skew against the argo-cd require.

17. [inferred] client-go 0.34 against the k3s v1.29.1 API server
    (`internal/infraconfig/defaults/platform.yaml`) exceeds the official ±1 version-skew
    policy — but so does today's 0.31 client, and this repo only uses stable core/apps/v1
    and dynamic/unstructured calls. Renovate PR #27 (k3s → v1.36.2-k3s1, pending
    dashboard approval) would land the server on the *other* side of the client at −2.
    No action in this migration.

## Alternatives considered

### Target v3.2.x instead of v3.4.5

v3.2 is the last minor whose `go.mod` targets go 1.25 and predates the gitops-engine
monorepo move (its require is the old, resolvable `github.com/argoproj/gitops-engine`
path) — no Go bump, no replace directive. Rejected: it re-opens a client/server minor
skew (server is 3.4.5) the moment it lands, contradicts the issue's acceptance criterion
("matching the deployed server minor"), and only defers the replace-directive dance to
the next bump. The Go 1.26 toolchain is already the local and CI reality.

### Satisfy gitops-engine with a direct `require` instead of a `replace`

Cleaner-looking (no replace block), but impossible: MVS picks the *maximum* of all
requires, and argo-cd's broken `v0.7.1-0.…` pseudo-version outranks every resolvable
`v0.0.0-…` pseudo-version (finding 5). Only `replace` overrides the selection.

### Jump the version-locked group to latest (k8s 0.36.3 / helm 3.21.3) in the same PR

What PR #28 proposes. Empirically does not compile against argo-cd v3.4.5 (finding 11).
Rejected on evidence; the Renovate caps in the Design section encode the same fact so
the bot stops proposing it.

### Silence the two deprecations with `//nolint:staticcheck` instead of fixing them

Smallest possible diff, but both fixes are equally small, verified behavior-neutral
(findings 8–9), and avoid carrying API surface upstream has already scheduled for
removal — which would otherwise resurface as a build break on the next major.

## Design

### A. go.mod

```
go 1.26.0

require github.com/argoproj/argo-cd/v3 v3.4.5   // replaces the /v2 require

// argo-cd v3.3+ vendors gitops-engine as an untagged nested module and pins it with a
// local-path replace that consumers don't inherit; the pseudo-version argo-cd requires
// does not resolve on the module proxy. Pin the nested module at the argo-cd release
// tag commit instead, and move this line in lockstep with the argo-cd/v3 require —
// resolve the new version with:
//   curl https://proxy.golang.org/github.com/argoproj/argo-cd/gitops-engine/@v/<tag-commit-sha>.info
replace github.com/argoproj/argo-cd/gitops-engine => github.com/argoproj/argo-cd/gitops-engine v0.0.0-20260709160802-564b94973b28
```

Then `go get helm.sh/helm/v3@v3.19.5` and `go mod tidy`. Expected post-tidy selections
(assert with `go list -m`): `argo-cd/v3 v3.4.5`, `k8s.io/{api,apimachinery,client-go}
v0.34.2`, `helm.sh/helm/v3 v3.19.5`, `sigs.k8s.io/structured-merge-diff/v6`,
`go-git/v5 v5.14.0`; `github.com/argoproj/gitops-engine` (old path) gone.

### B. Import rewrite

`github.com/argoproj/argo-cd/v2/...` → `github.com/argoproj/argo-cd/v3/...` in the six
files from finding 1. Nothing else changes in five of them; `applications.go` also gets
the two fixes below. While `install_test.go` is open, update the fake version string
`"v2.99.0-test"` → `"v3.99.0-test"` (cosmetic; the value is asserted nowhere).

### C. Deprecation fixes (both in `internal/argocd/grpcclient/applications.go`)

1. In `toAppStatus` (line 62), drop the `Message: app.Status.Health.Message` field
   initializer. The `AppStatus.Message` field itself **stays** — the grpcsync grace-period
   path still sets a synthetic message on its `Result`, and removing the field would
   cascade through `Result`/display plumbing for no benefit. Against a v3 server the
   app-level source is always empty anyway (finding 8).
2. In `RunResourceAction` (line 301), call `client.RunResourceActionV2` with
   `applicationpkg.ResourceActionRunRequestV2{...}` — field-for-field identical to the
   current request (finding 9).

### D. Config cleanup

Delete the line `application.resourceTrackingMethod: annotation` from
`internal/infraconfig/defaults/argocd-values.yaml` (`configs.cm`). Default since v3.0
(finding 14). No other values-file entry is affected by the v3 server (already deployed
and healthy today).

### E. renovate.json

> **Superseded in part — see Deviation 1.** The kubernetes-group cap design below was
> not implemented; the group is held with `dependencyDashboardApproval` and no
> `allowedVersions`. The rest of this section landed as written.

Replace the single kubernetes rule (`renovate.json:25-34`) with three rules sharing
`"groupName": "kubernetes"` (one PR, per-package caps — `allowedVersions` is
per-rule, and structured-merge-diff's v6.x would violate a shared `<0.35` cap):

```json
{
  "description": "k8s.io libs are version-locked to argo-cd v3.4 (k8s 0.34.x). 0.35+ removes API packages that argo-cd's k8s.io/kubernetes internals still import (verified: 0.36 does not build); raise the cap only alongside an argo-cd bump",
  "groupName": "kubernetes",
  "matchManagers": ["gomod"],
  "matchPackageNames": ["k8s.io/**"],
  "allowedVersions": "<0.35.0"
},
{
  "description": "helm 3.19.x is the k8s-0.34 partner (helm 3.N pairs with k8s 0.(N+15)); its cap moves with the k8s cap above",
  "groupName": "kubernetes",
  "matchManagers": ["gomod"],
  "matchPackageNames": ["helm.sh/helm/v3"],
  "allowedVersions": "<3.20.0"
},
{
  "description": "structured-merge-diff moves with the kubernetes group; no cap needed",
  "groupName": "kubernetes",
  "matchManagers": ["gomod"],
  "matchPackageNames": ["sigs.k8s.io/structured-merge-diff/**"]
}
```

The `<0.35.0` cap is safe for the k8s.io direct deps: `api`/`apimachinery`/`client-go`
sit at 0.34.x and `utils` uses `v0.0.0-…` pseudo-versions; indirect k8s.io modules with
larger versions (e.g. klog v2.130.x) are not updated by Renovate's gomod manager by
default. The existing `k8s.io/kubernetes` disable rule stays as is.

Replace the argo-cd major-disable rule (`renovate.json:40-45`) with an all-updates hold:

```json
{
  "description": "argo-cd client must track the deployed server (chart pin in internal/infraconfig/defaults/argocd.yaml): bump by hand together with the chart, move the go.mod gitops-engine replace to the new tag commit, and raise the kubernetes caps if argo-cd's k8s minor moved. Majors additionally change import paths",
  "matchPackageNames": ["github.com/argoproj/argo-cd/v3"],
  "dependencyDashboardApproval": true
}
```

Add a disable rule for the replace target (finding 16):

```json
{
  "description": "Replace-pinned in go.mod to the argo-cd release commit; must never move independently of github.com/argoproj/argo-cd/v3",
  "matchPackageNames": ["github.com/argoproj/argo-cd/gitops-engine"],
  "enabled": false
}
```

Comment-only updates: the go-git hold description (the securejoin conflict is gone —
finding 12 — but upstream argo-cd caps go-git at 5.14 pending go-git/go-git#1551, so
the hold itself stays); the "argoproj pseudo-versions" group drops
`github.com/argoproj/gitops-engine` from `matchPackageNames` (finding 13); the k3d hold
description's "module targets 1.25" clause becomes "module targets 1.26.0 (k3d v5.9
wants ≥1.26.3)" — the docker/docker v28 half of that hold still applies.

### F. Docs

`CLAUDE.md:22` "Go 1.25." → "Go 1.26."; `CLAUDE.md:48` "`grpcclient` wraps
`argo-cd/v2/pkg/apiclient`" → `argo-cd/v3/pkg/apiclient`. Historical documents under
`docs/superpowers/` mentioning v2 stay untouched (they describe the state of their
time). Contract tests are unaffected: no command-tree or MCP-tool-list changes.

## Implementation plan

1. `go.mod`: set `go 1.26.0`; swap the argo-cd require to `/v3 v3.4.5`; add the
   gitops-engine replace with its comment (Design A).
2. Rewrite imports `/v2` → `/v3` in the six files (Design B), incl. the cosmetic test
   string.
3. `go get helm.sh/helm/v3@v3.19.5 && go mod tidy` → verify `go build ./...`.
4. Apply the two deprecation fixes in `grpcclient/applications.go` (Design C) → verify
   `make lint` is clean.
5. Drop `application.resourceTrackingMethod` from `argocd-values.yaml` (Design D).
6. Rework `renovate.json` per Design E; validate with a **pinned** validator, e.g.
   `npx --yes --package renovate@<current> -c 'renovate-config-validator renovate.json'`
   (a stale npx cache has produced phantom errors before — never run it unpinned).
7. Update the two `CLAUDE.md` lines (Design F).
8. `make build && make test && make lint`; assert module selections with
   `go list -m github.com/argoproj/argo-cd/v3 k8s.io/client-go helm.sh/helm/v3`.
9. Real-cluster acceptance run (Test plan below) on the user's machine.
10. After merge: PR #28 should get re-created/rebased by Renovate against the caps (or
    close it and let the next scheduled run open a capped one); PR #48 (helm v4) stays —
    it is a separate module-path migration, explicitly out of scope here.

## Deviations

1. **Design E (renovate.json kubernetes group) — changed by user decision.** The spec
   proposed splitting the group into three rules carrying `allowedVersions` caps
   (`<0.35.0` / `<3.20.0`); Risk #1 flagged the cap style as an open question. The user
   chose `dependencyDashboardApproval` on the existing single grouped rule instead, with
   no caps, on the stated goal of keeping the app safe *while still seeing that updates
   exist*. The deciding fact: `allowedVersions` is a lookup-time filter, so capped
   versions never become update objects and vanish from the Dependency Dashboard
   entirely — safety by invisibility. The approval gate provides the same protection
   while leaving k8s 0.36.x / helm 3.21.x visible, and the rule description now names the
   real ceiling (0.34.x / 3.19.x) and the reason at the point of decision.
   The rest of Design E (argo-cd rule retarget + all-updates hold, new
   `argo-cd/gitops-engine` disable rule, comment-only updates) landed as specified.

2. **`toAppStatus` carries an explanatory comment.** Design C.1 said only "drop the
   `Message:` field initializer". A three-line comment was added recording *why* the
   field is deliberately left unset, so the absence doesn't read as an oversight to the
   next reader.

3. **Incidental MVS floor raises**, not mentioned in the spec: `fatih/color`
   1.16.0→1.18.0, `sirupsen/logrus` 1.9.3→1.9.4, `golang.org/x/term` 0.39.0→0.44.0,
   `google.golang.org/grpc` 1.78.0→1.79.3. Verified via `go mod graph` that each is a
   floor required by argo-cd v3.4.5 and/or helm 3.19.5 — forced, not opportunistic.

4. **`go list -m all` is unusable in this repo, so the Test-plan check for the departed
   old-path module was run differently.** It fails on `k8s.io/kubernetes`'s v0.0.0
   staging deps (`k8s.io/cloud-provider@v0.0.0: invalid version` and ~13 more). Confirmed
   pre-existing, not a regression: an identical run in a `git worktree` at pristine HEAD
   fails the same way — and it is the exact condition `renovate.json`'s `k8s.io/kubernetes`
   disable rule already documents. Substituted
   `go list -m github.com/argoproj/gitops-engine`, which answers the question directly:
   `module github.com/argoproj/gitops-engine: not a known dependency`. Also confirmed
   absent from both `go.mod` and `go.sum`.

## Test plan

Verifiable in a session (no Docker) — **all executed, all pass:**

- ✅ `make build` (exit 0), `make test` (exit 0 — 17 packages `ok` under `-race`,
  0 failures), `make lint` (exit 0 — golangci-lint "0 issues", confirming both
  Design-C SA1019 fixes landed).
- ✅ `go list -m` selections match Design A exactly: `argo-cd/v3 v3.4.5`,
  `k8s.io/{api,apimachinery,client-go} v0.34.2`, `helm.sh/helm/v3 v3.19.5`,
  `sigs.k8s.io/structured-merge-diff/v6 v6.3.2`, `go-git/v5 v5.14.0`.
- ✅ Old-path `github.com/argoproj/gitops-engine` gone from the module graph
  (see Deviation 4 for the substituted command).
- ✅ Contract tests `cmd/sikifanso/command_structure_test.go` and
  `internal/mcp/server_test.go` pass **unmodified** — neither the command tree nor the
  25-tool list was touched.
- ✅ `npx --yes --package renovate@41.140.1 -c 'renovate-config-validator renovate.json'`
  → "Config validated successfully".
- ✅ `install_test.go`'s `TestWaitForGRPC_*` drives the migrated v3 apiclient against a
  live in-process gRPC listener — real wire-level coverage, not just compilation.
- ✅ Finding 3's landmine confirmed handled: `go get` resolved cleanly *with* the
  replace in place, where the spec predicted a bare `go get` would fail outright.

**Manual verification COMPLETE** — executed against real Docker/k3d on 2026-07-26,
ArgoCD server v3.4.5, all passing:

- ✅ `cluster create` — full run green. `WaitForGRPC` resolved in **5 ms**
  (`12:13:50.645 waiting for ArgoCD gRPC server {"addr":"localhost:60027"}` →
  `12:13:50.650 ArgoCD gRPC server is ready`), infra apps `cilium` + `argocd` both
  Synced/Healthy, all three ApplicationSets applied.
- ✅ `app enable valkey` (12.3 s) → `app disable valkey` (2.0 s) — exercises `OpEnable`
  (appset annotate → CR poll → watch), tier sequencing (`watching tier {"tier":"1-data"}`),
  and reverse-tier disable over the v3 gRPC API.
- ✅ `agent create testagent` (5.9 s) → `agent delete testagent` (63 s) — agents
  ApplicationSet refresh path.
- ✅ `cluster stop` → `cluster start` — clean; port mappings preserved.
- ✅ `app status argocd` — full resource tree over session-authenticated v3 gRPC (0.69 s).
- ✅ Design D regression check: `kubectl get cm argocd-cm -n argocd -o jsonpath=...`
  returns **empty** (server default = annotation), and enabled apps still reach Healthy.
- ✅ Only five host ports are now mapped on the k3d LB, and `argocd-server` exposes
  NodePorts 30080/30443 only — no phantom gRPC port.

Two defects were found and fixed during this run; both **predate** the v3 migration
(see the Follow-up defects section below).

Original checklist, for the record:

- `sikifanso cluster create` → ArgoCD v3.4.5 installs, `WaitForGRPC` succeeds, infra
  apps reach Healthy (exercises apiclient + session auth + watch streaming + tier
  sequencing).
- `sikifanso app enable <x>` → `sikifanso app disable <x>` — exercises `OpEnable`
  (appset annotate → CR poll → watch) and reverse-tier disable over the v3 gRPC API.
- `sikifanso app rollback` or MCP `argocd_rollback` — exercises the application service
  mutation path.
- `sikifanso agent create/delete` — exercises the agents ApplicationSet refresh path.
- Tracking-method regression check after Design D: on a fresh cluster,
  `kubectl get cm argocd-cm -n argocd -o jsonpath='{.data.application\.resourceTrackingMethod}'`
  should be empty (server default = annotation), and enabled apps still sync to Healthy.

## Risks & open questions

All four are closed. Resolutions below reflect what shipped, not what this spec originally
proposed — where the two differ, the reversal and its reason are recorded.

1. **Renovate hold style — resolved, reversing this spec's own design.** The design proposed
   `allowedVersions` caps. **No caps shipped**: every hold is `dependencyDashboardApproval`.
   `allowedVersions` filters at *lookup* time, so a capped release never becomes an update
   object and vanishes from the Dependency Dashboard entirely — safety by invisibility, which
   hides exactly the information the dashboard exists to surface. The approval gate protects
   the build just as well while leaving newer releases visible; the human decides at the
   checkbox. The real ceiling and its reason live in each rule's `description`, since that
   text is what gets read at the moment of deciding.

   The maintenance duty this section previously described — "every future argo-cd bump that
   moves its k8s minor must also move both caps" — **does not exist**. There are no caps to move.

2. **client-go 0.34.2 vs k3s v1.29.1 skew — accepted, with a sequenced follow-up.** Note this
   migration *widened* the skew rather than inheriting it: argo-cd v2.14.20 put client-go at
   0.31.x against k3s 1.29, and v3.4.5 moves it to 0.34.2. That pairing is nonetheless the one
   the acceptance run actually exercised, which the alternative (landing k3s first) would not be.
   Merge order is therefore #61 first, then PR #27 (`rancher/k3s` → `v1.36.3-k3s1`) as the
   immediate next change — not queued behind other work, since main carries the wide skew until
   it lands.

3. **gitops-engine replace hygiene — resolved, now guarded.** This section previously judged a
   machine check to be over-engineering, assuming it needed a doctor check or a network call.
   It needs neither. The go command already records the tag's commit in the module cache, so
   the invariant is checkable offline: `TestGitopsEnginePinMatchesArgoCDTag`
   (`internal/argocd/gitopsengine_pin_test.go`) reads the argo-cd/v3 version and the
   gitops-engine replace out of `go.mod`, then asserts the pinned commit prefixes the
   `Origin.Hash` recorded for that argo-cd tag. It skips when the proxy did not report
   `Origin`, and `TestVerifyPinDetectsMismatch` proves it can fail — including against
   `97ad5b59a627`, the stale placeholder someone would copy from argo-cd's own `require` line.

   Verified for this bump: the replace `v0.0.0-20260709160802-564b94973b28` decodes exactly to
   the `v3.4.5` tag commit `564b94973b284b8de98da7cee6eeade2cb941e46` (`2026-07-09T16:08:02Z`).

   A doctor check would have been the wrong home regardless: those run against a live cluster,
   and this is a build-time invariant.

4. **Fresh-cluster-only validation — verified, and the reasoning corrected.** The original
   argument (the deployed v3 server accepts calls from both client majors) holds, but it
   missed the sharper question raised by the phantom-port fix: pre-existing sessions were
   written with a `grpcAddress` pointing at NodePort 30084, which this change deletes.

   That turns out to be harmless, for a reason worth recording: **nothing ever read the stored
   address.** `grpcClientFromSession` (`cmd/sikifanso/middleware.go`) passes
   `sess.Services.ArgoCD.URL` to `grpcclient.FromSessionCreds`, which parses the host from it.
   The field was write-only on this branch and on `main` alike. It has now been removed
   (below), which is backward-compatible: `session.Load` uses `sigs.k8s.io/yaml` → non-strict
   `json.Unmarshal`, so existing `session.yaml` files carrying `grpcAddress:` load with the key
   ignored. No migration step.

## Follow-up defects found during acceptance testing

All are independent of the v3 migration (the chart pin, `ports.go` and `platform.yaml`
were untouched by it) and were fixed on this branch with the user's approval.

### 1. Phantom ArgoCD gRPC NodePort — `cluster create` hung forever

`ArgoCDRuntimeOverrides` injected `server.serviceGrpc.nodePortGrpc`, but the argo-cd
Helm chart has **no `server.serviceGrpc` key** — verified absent from every chart version
7.7.5 → 10.2.1. Helm drops unknown values silently, so the gRPC Service was never created
and NodePort 30084 never existed. k3d still mapped a host port to it, and its load
balancer accepted the TCP connection with no upstream, so the HTTP/2 handshake never
completed. Introduced in `56194a5` (2026-03-29).

ArgoCD multiplexes gRPC and REST on the server's single HTTP port (the values file already
passes `--insecure`), so the separate port was never needed. Fix: removed the invented
chart key and the phantom port end to end — host port allocation 6 → 5, the 30084 mapping
dropped, `GRPCAddress` now points at the UI port. Guarded by
`TestArgoCDRuntimeOverridesUsesOnlyRealChartKeys`, which asserts we never inject a key the
chart does not define — the failure mode was silent, so a test on rendered output would
not have caught it.

### 2. `WaitForGRPC` did not honour its own timeout

`apiclient.NewClient` is not a cheap constructor: it eagerly probes the Version endpoint
to decide whether gRPC-web is needed, and that probe hardcodes `context.Background()`
upstream (`NewVersionClient` → `newConn` → `waitForReady` → `WaitForStateChange`).
`NewVersionClient` inside the retry loop has the same problem. A *refused* connection
fails fast — which is why the pre-existing `TestWaitForGRPC_Timeout` passed — but an
address that accepts TCP and never speaks blocked indefinitely (measured: >5 min against
a 12 s context). Fix: race both client construction and each probe against ctx.
`TestWaitForGRPC_UnresponsiveListener` covers it with a black-hole listener and is itself
bounded, so a regression fails the test instead of stalling the suite.

### 3. Same unbounded dial in `grpcclient` — fixed

`grpcclient.NewClient` (`internal/argocd/grpcclient/client.go:51,77`) calls
`apiclient.NewClient` twice, plus `NewSessionClient()`, all unbounded, and its callers
(`cmd/sikifanso/middleware.go:70`, `cluster_create.go:111`, `internal/mcp/argocd.go:30`,
`internal/mcp/doctor.go:70,91`) pass a context with no deadline. Observed during
acceptance testing: `app status argocd` run seconds after `cluster start`, while
argocd-server was still coming up, hung for **10 minutes** with no output before being
killed; it returned in 0.69 s once the pod settled. Every `app`/`agent`/MCP command uses
this path. Because no caller supplies a deadline, a ctx race alone would not help — the
bound has to live in `NewClient`. Fixed with a `dialTimeout` var (mirroring
`grpcReadyTimeout`): the blocking sequence moved into `connect()` and raced against it, so
an unreachable server reports `timed out connecting to ArgoCD gRPC at <addr> after 30s`.
Covered by `TestNewClient_UnresponsiveListener` (black-hole listener, itself bounded;
returns in 2.00 s against a 2 s timeout).

### 4. Docker Desktop virtiofs dentry cache broke back-to-back cluster creates — fixed

Creating a cluster within an hour of a previous one failed reproducibly (observed twice,
10:51 and 10:59) with `error while creating mount source path '/host_mnt/.../gitops'`,
surfacing as an opaque `node k3d-default-server-0 failed to get ready … status=restarting`.

Root cause is in Docker Desktop, not this repo, but it is triggered deterministically by
the create flow. Docker Desktop shares `/Users` into its VM over virtiofs with
`entry_timeout=3600,attr_timeout=3600`, and nothing invalidates the guest dentry cache
when the host unlinks a path. `cluster create` deletes and re-scaffolds the gitops dir, so
the VM serves the previous cluster's cached, deleted dentries for up to an hour. k3d then
copies its entrypoint scripts into the *created-but-not-started* server container, and the
daemon's copy-to-stopped-container path resolves bind sources through that stale cache —
whereas `docker start` resolves them host-side and succeeds. The container therefore starts
without its entrypoint and crash-loops (`exec /bin/k3d-entrypoint.sh failed`). This also
explains why ad-hoc `docker run -v` probes of the same path all succeeded: the start path
heals the cache.

Fix (`internal/cluster/prewarm.go`): start a throwaway container with the same bind after
scaffolding and before `ClusterRun`, which heals the cache. Reuses the already-present k3s
image and never pulls; skipped on native Linux; best-effort with debug-only logging, since
a genuine mount failure surfaces moments later in `ClusterRun` with a better message.
Verified: the delete-then-create cycle that failed twice now completes, with all three
`WriteFileAction` preStart hooks executing cleanly. Worth reporting upstream to Docker
Desktop — tracked as its own issue so the justification for `prewarm.go` outlives this spec.

## Found during merge-readiness review

### 5. Vestigial `GRPCAddress` — the phantom port's last limb

Defect 1 removed the invented chart key and the phantom port end to end, but left
`Session.GRPCAddress` behind: written at `internal/cluster/cluster.go`, persisted in
`session.yaml`, and read by **nothing** — neither on this branch nor on `main`. Its
round-trip test still asserted `localhost:30084`, so the suite documented the correctness of
a port the same branch had just proved never existed.

Harmless at runtime (see risk 4 above — callers resolve the gRPC host from
`Services.ArgoCD.URL`), but actively misleading to anyone reading the session format or that
test. Removed: the field, its write, and `TestSessionRoundTrip_GRPCAddress`. Backward-compatible
for existing sessions, which simply carry an ignored key.

## Acceptance re-run on the final HEAD (2026-08-08)

The results recorded earlier in this spec predate two commits — `0514953` (grpcclient dial
timeout) and `31b5dcc` (virtiofs prewarm) — both of which touch the create/connect path that
run was exercising. Re-run in full against `bin/sikifanso` built from the final branch HEAD.
Not via the MCP tools: `.mcp.json` resolves `sikifanso` from `PATH` to a Homebrew v0.9.1
release, so an MCP-driven run would have exercised the pre-migration binary.

| # | Step | Result |
|---|------|--------|
| 1 | `cluster create -c v3check` | exit 0 |
| 2 | `app status argocd` right after create | **0.76 s** |
| 3 | `app enable postgresql` | exit 0, 88.6 s |
| 4 | `cluster doctor` | 8/8 checks ok |
| 5 | `cluster stop` → `cluster start` | exit 0 / exit 0 |
| 6 | `app status argocd` right after start | **failed — 30 s timeout** → defect 6, fixed below |
| 7 | `cluster delete` → immediate `cluster create` | exit 0 / exit 0, 1 m 40 s |
| 8 | `cluster delete` | exit 0, no containers left |

Step 6 was fixed on this branch and the whole sequence re-run; see the defect-6 write-up and
the final results table at the end of this section.

Step 3 confirms the dependency cascade and tier sequencing: `postgresql` auto-enabled
`cnpg-operator` and `prometheus-stack`, and the sync watched tier `0-operators` to completion
(14:09:28 → 14:10:27) before starting `1-data`. All three reached Synced/Healthy.

Step 7 confirms defect 4 is fixed — the delete-then-create cycle that previously failed
reproducibly now completes with no `error while creating mount source path`.

### Step 6 (defect 6): `WaitForGRPC` could never observe a server that came up late

`app status argocd` run immediately after `cluster start` failed at exactly the dial timeout:

```
Error: timed out connecting to ArgoCD gRPC at localhost:56415 after 30s
```

Retried ~15 s later it returned in 0.70 s. The obvious reading — "the 30 s budget is a little
short", i.e. #54 — is wrong, and chasing it produced the useful evidence. Raising the wait
budget 30 s → 90 s → 180 s changed nothing: each run timed out at *exactly* the budget while a
freshly built client connected in 1 s. A wait that fails for any budget is not a slow server.

Two compounding defects in `WaitForGRPC`:

1. **Stale client.** `apiclient.NewClient` was called once *before* the retry loop. It eagerly
   probes Version to decide whether gRPC-web is needed and bakes that decision in, so a client
   built while ArgoCD was still down never recovers — the loop then retried a permanently
   broken client until the budget expired. On `cluster start` that is the normal case.
2. **Unbounded attempts.** Each probe was raced only against the *overall* context. Against
   k3d's load balancer — which accepts TCP with no upstream while ArgoCD starts — the first
   probe blocks forever, absorbing the entire budget, so no retry ever ran.

Fixed: build a fresh client per attempt, and bound each attempt with `grpcProbeTimeout` (5 s)
so a stalled one is abandoned and retried. `grpcReadyTimeout` also raised to 180 s — a ceiling,
not a target, since the wait returns as soon as the server answers.

A third gap sat underneath: `cluster.Start` never called `WaitForGRPC` at all, on this branch
or on `main`. `k3dclient.ClusterStart{WaitForServer: true}` waits for the **k3s API server**,
not ArgoCD, so `cluster start` reported success while the next command could not connect.
`Create` has always waited; `Start` now does too, non-fatally — the cluster is started either
way, and failing a running cluster would be worse than a warning.

Measured after the fix, on a cluster with three catalog apps: the wait reports
`ArgoCD gRPC server is ready` after 35 s, `cluster start` returns in 47 s, and `app status`
immediately afterwards succeeds in 1 s.

Why the existing tests missed it: `TestWaitForGRPC_DelayedStart` closes the port, so dials are
*refused* and fail fast, leaving the client healthy. The production condition is a black hole
that accepts TCP and never answers, which poisons the client instead.
`TestWaitForGRPC_BlackHoleThenReady` now models that — and it earned its keep by failing
against the first attempted fix, proving fresh clients alone were not sufficient.

### Final acceptance run — all steps pass

Full sequence re-run against a binary built from the branch HEAD with the defect-6 fixes:

| # | Step | Result |
|---|------|--------|
| 1 | `cluster create -c v3check` | exit 0 |
| 2 | `app status argocd` right after create | exit 0, 1 s |
| 3 | `app enable postgresql` | exit 0; `0-operators` 15:01:46 → `1-data` 15:02:44 |
| 4 | `cluster doctor` | exit 0, 8/8 ok |
| 5 | `cluster stop` → `cluster start` | exit 0 / exit 0, 40 s |
| 6 | `app status argocd` right after start | **exit 0, 3 s** |
| 7 | `cluster delete` → immediate `cluster create` | exit 0 / exit 0, 100 s |
| 8 | `cluster delete` | exit 0, 0 containers left |

The `cluster start` in step 5 logged `ArgoCD gRPC server is ready` after 28 s — the wait now
observes readiness instead of exhausting its budget, and `start` returns only once the next
command can connect.
