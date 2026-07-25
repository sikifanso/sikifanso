# Renovate changes for `sikifanso-homelab-bootstrap`

**Date:** 2026-07-25
**Status:** Pending — to be applied by hand in the bootstrap repo
**Companion to:** the Renovate config landed in this repo (`renovate.json`)

## Context

The CLI repo now has Renovate (`renovate.json` at the repo root) covering `gomod`,
`github-actions`, and three custom managers for versions that live outside a manifest.
The bootstrap repo has run Renovate since 2026-03 via a **self-hosted GitHub Action**
using a PAT, and needs targeted fixes rather than a greenfield setup.

This spec was written from an investigation of that repo, but **could not be re-verified
at authoring time** — that repo was not checked out and the sandbox blocked
`github.io` / `api.github.com`. Treat the specifics below (file contents, dep counts,
PR numbers) as needing confirmation against the live repo before acting. The *reasoning*
holds regardless.

## Prerequisite

That repo had uncommitted working-tree changes (`catalog/{langfuse,postgresql,temporal}.yaml`,
deleted `cnpg-operator.yaml`) and open PR #55. Land or stash those first — item 2 below
should be written against the *post-change* state, since that is what makes it necessary.

---

## 1. Migrate to the hosted Mend Renovate GitHub App

Install the hosted app **before** deleting anything, so there is never a window with no
automation. Then delete `.github/workflows/renovate.yml` and remove the `RENOVATE_TOKEN`
secret.

Three things this fixes for free, all of which appear as warnings in every current run:

- `WARN: Cannot access vulnerability alerts. Please ensure permissions have been granted.`
  The PAT cannot read Dependabot alerts, so **security-driven updates do not work at all**
  today. Granting the app that permission makes `vulnerabilityAlerts` start functioning.
- `WARN: Using the default gitAuthor email address … renovate@whitesourcesoftware.com is
  not recommended` — every Renovate commit currently lands `Unverified`.
- PRs are authored by a personal account (the PAT owner) rather than a bot.

All three repos are public, so they qualify for the enhanced OSS tier. The headroom is
worth having: the CLI repo's dependency graph is ~245 indirect modules with a 113 KB
`go.sum`, which is marginal on the base tier.

## 2. Close the OCI gap before it opens

The existing `customManager` regex requires `repoURL: https?://`. On `main` all 19 catalog
entries match, and the last run logged `"regex": {"fileCount": 19, "depCount": 19}`.

The pending working-tree change migrates `catalog/postgresql.yaml` to
`repoURL: registry-1.docker.io/cloudpirates`, chart `postgres` — an **OCI registry**. The
moment that is committed, the entry silently drops out of coverage (19 → 18 deps) with no
error and no PR. Add the OCI manager as part of this work so the gap never opens.

```json
{
  "customType": "regex",
  "description": "Catalog entries on OCI registries (no https:// scheme)",
  "managerFilePatterns": ["/^catalog/[^/]+\\.yaml$/"],
  "matchStrings": [
    "repoURL:\\s*(?<registryUrl>[^:\\s]+)\\s*\\nchart:\\s*(?<depName>\\S+)\\s*\\ntargetRevision:\\s*\"?(?<currentValue>[^\"\\s]+)\"?"
  ],
  "datasourceTemplate": "docker"
}
```

**Why `[^:\s]+` and not a negative lookahead.** Renovate's regex managers run on **RE2**
(via `uhop/node-re2`), which does not support lookahead or backreferences — a
`(?!https?://)` guard is rejected outright. Excluding `:` from the character class achieves
the same mutual exclusion: OCI coordinates like `registry-1.docker.io/cloudpirates` contain
no colon, while `https://…` fails at the `:` immediately, so the two managers cannot both
claim the same entry.

Two RE2 gotchas while writing these:

- Matching is **per-file, not per-line** — `^` and `$` anchor to the whole file. Use
  `(?:^|\r\n|\r|\n|$)` if a line boundary is genuinely needed.
- `managerFilePatterns` are *path* patterns and follow different rules from `matchStrings`.

Also make the existing HTTPS manager less order-brittle: it currently demands
`repoURL` / `chart` / `targetRevision` on strictly consecutive lines, so it silently stops
matching if a key is reordered or inserted.

**Must not match:** `bootstrap/root-{app,catalog,agents}.yaml` contain
`targetRevision: "{{targetRevision}}"` and `targetRevision: HEAD` — ApplicationSet
templates, not dependencies.

One thing the dry run must settle rather than assume: whether the `docker` datasource wants
`registryUrl` as the bare host (`registry-1.docker.io`) with the namespace folded into
`depName` (`cloudpirates/postgres`), or accepts the host+path split above. If neither
resolves, fall back to the inline `# renovate:` marker approach used in the CLI repo —
more verbose, but unambiguous, and proven to extract correctly there.

## 3. Gate the existing automerge

That repo has **no CI at all** beyond Renovate itself, yet auto-merges patch+minor chart
bumps for user-facing infrastructure. Add a lightweight `.github/workflows/validate.yml`:

- YAML parse of `catalog/`, `agents/`, `apps/`
- schema check that each catalog entry carries the fields `catalog.Entry` expects:
  `name`, `repoURL`, `chart`, `targetRevision`, `namespace`, `enabled`, `tier`

Then keep `automerge` on patch+minor — now actually gated.

Top-level keys to add alongside, mirroring the CLI repo:

```json
{
  "dependencyDashboard": true,
  "timezone": "Europe/Berlin",
  "schedule": ["before 6am on monday"],
  "semanticCommits": "enabled",
  "prConcurrentLimit": 3,
  "labels": ["dependencies"]
}
```

`dependencyDashboard` is the important one — see item 4.

## 4. Five catalog entries point at Helm repos that do not exist

The most consequential find, and a correctness bug rather than a Renovate one. All five of
these `repoURL`s return **HTTP 404** — no `index.yaml`, no repository:

| Catalog entry | `repoURL` |
|---|---|
| `guardrails-ai` | `guardrails-ai.github.io/helm-charts` |
| `nemo-guardrails` | `nvidia.github.io/NeMo-Guardrails` |
| `presidio` | `microsoft.github.io/presidio` |
| `unstructured` | `unstructured-io.github.io/helm-charts` |
| `text-embeddings-inference` | `huggingface.github.io/helm-charts` |

They are placeholders with invented URLs and a `0.1.0` version. **`app enable` on any of
them cannot deploy** — ArgoCD will fail to resolve the chart.

Renovate has reported this every single run and nobody saw it. From the 2026-07-20 log:

```
WARN: Package lookup failures
  "Failed to look up helm package guardrails-api: no-result",
  "Failed to look up helm package nemo-guardrails: no-result",
  "Failed to look up helm package presidio: no-result",
  "Failed to look up helm package text-embeddings-inference: no-result",
  "Failed to look up helm package unstructured-api: no-result"
```

The workflow still exits `success` because lookup warnings are not fatal, and there is no
dependency dashboard to surface them. Enabling `dependencyDashboard` (item 3) is the
mechanism that would have made this visible on day one.

Decide per entry: find the real chart repo, or drop the entry. Until then, mark them
`ignoreDeps` so the warnings stop being noise.

Separately — real charts, just drifting: `litellm-proxy` is pinned `0.0.4` against a repo
whose latest is `0.2.0`, and `kube-prometheus-stack` sits at `82.15.1` while a `v86` PR was
autoclosed (#46).

---

## Verification

```bash
npx --yes --package renovate -- renovate-config-validator renovate.json
LOG_LEVEL=debug npx --yes renovate --platform=local --dry-run=extract
```

Note: `renovate --platform=local` reads the config from **committed** git state, not the
working tree. An uncommitted `renovate.json` is silently ignored and Renovate falls back to
onboarding defaults — which looks like a passing run with no custom managers. Commit first,
then extract.

Expect every catalog entry to extract, **including the OCI one** — that entry is what proves
item 2 works. The baseline to beat is the current `depCount: 19`; after the CloudPirates
change lands it must still be 19, not 18.

Then confirm the 404s are *handled*, not merely rediscovered: a run must produce **zero**
`Failed to look up helm package` warnings, either because the entries were corrected,
dropped, or explicitly ignored. A run that still logs them means item 4 was skipped.

Finally, open a throwaway PR that breaks a catalog entry's schema and confirm `validate.yml`
fails, so automerge cannot slip past it.

## Cross-repo invariant

A tagged CLI release clones the bootstrap repo **at the same tag**
(`resolveBootstrapVersion` in `cmd/sikifanso/cluster_create.go`). Renovate does not touch
this, but chart bumps merged into the bootstrap repo only reach released CLI users at the
next matching tag pair.
