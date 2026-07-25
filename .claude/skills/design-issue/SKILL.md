---
name: design-issue
description: Phase 1 of the issue workflow — deeply investigate a GitHub issue, do upstream research, and produce a committed design spec in docs/specs/. Produces NO code changes. Use when the user says "design issue N", "/design-issue N", or asks for an investigation/design for an issue. Intended for a high-reasoning model session (Fable).
---

# Design an issue (Phase 1: investigate → research → design)

You are the **design engineer**. Your deliverable is a spec document, not code.
A different session (possibly a different model) will implement it later with no
memory of your investigation — everything they need must be in the spec.

**Hard rule: no production code changes in this phase.** Throwaway probes in the
scratchpad are fine; the only repo change you commit is the spec file.

## Inputs

`$ARGUMENTS` is the issue number (e.g. `53`). If missing, ask for it.

## Step 1 — Understand the issue

1. Fetch the issue from `sikifanso/sikifanso` via the GitHub MCP tools, including comments.
2. Fetch anything it links: related issues, Renovate PRs, prior specs in `docs/specs/`
   and `docs/superpowers/specs/`.
3. Restate the problem in your own words. If the issue's premise looks wrong, say so
   in the spec — a design that corrects the issue is more valuable than one that
   follows it off a cliff.

## Step 2 — Verify every claim against the code

Issues go stale. For each factual claim (file, line, behavior, version):

- Re-find it in the current default branch and record `file:line` evidence.
- Where feasible, demonstrate the behavior: read the call path end-to-end, run
  `make build` / `make test`, write a scratchpad probe. Distinguish clearly between
  **verified** facts and **inferred** ones — the spec must label which is which.

## Step 3 — Upstream research (when the issue touches k3d / Cilium / ArgoCD / k3s / Helm)

- Check the pinned version in this repo first (`go.mod`, `internal/infraconfig/defaults/`),
  then read upstream docs/changelogs **for that version and the target version**.
- Record every load-bearing external fact with a URL and the version it applies to.
  "Cilium supports X" is useless to the implementer; "Cilium ≥ 1.16 supports X
  (link), we pin 1.19.6" is actionable.
- Prefer primary sources (upstream docs, changelogs, source code) over blog posts.

## Step 4 — Design

- Develop **at least two** credible approaches. For each: how it works, what changes,
  failure modes, blast radius, cross-repo impact (`sikifanso-homelab-bootstrap`
  invariants are listed in CLAUDE.md).
- Pick one and justify the choice against the alternatives — the implementer should
  never wonder "why not the obvious other way?".
- Think like an operator: what happens on `cluster stop/start`, on a slow machine, on
  re-run after partial failure, on the next version bump?

## Step 5 — Write the spec

Create `docs/specs/<today>-issue-<N>-<slug>.md` following the house style of the
existing specs in that directory. Required structure:

```markdown
# <Title>

**Date:** YYYY-MM-DD
**Status:** Draft
**Issue:** sikifanso/sikifanso#<N>

## Problem
What is broken/missing, in current-code terms. Verified evidence as `file:line`.

## Findings
Numbered, self-contained facts from investigation + research. Label each
[verified] or [inferred]. External facts carry a URL + applicable version.

## Alternatives considered
One subsection per approach, with the concrete reason it lost.

## Design
The chosen approach, in enough detail that an implementer never has to guess
an intent. Include exact values/names/config where known.

## Implementation plan
Ordered checklist of changes: file → what changes → why. Call out contract
tests that must be updated (command tree, MCP tool list) and CLAUDE.md
sections that become stale.

## Test plan
How each acceptance criterion from the issue will be verified. Separate
"verifiable by `make test` in a session" from "needs a real Docker/k3d run
on the user's machine" — remote sessions cannot run k3d clusters.

## Risks & open questions
Anything the human should decide before implementation starts.
```

## Step 6 — Hand off

1. Commit **only the spec** with message `docs: design spec for issue #<N> — <slug>`
   (no signature lines, per CLAUDE.md) and push to the session's designated branch.
2. Comment on the issue: 3–5 sentence summary of the chosen design, the branch and
   spec path, and any open questions that block approval. End the comment with the
   Claude Code attribution footer.
3. Tell the user the spec is ready for review, and that the next step is flipping
   **Status: Draft → Approved** (they can just say "approved" and let the
   implementation session flip it) before running `/implement-design <N>`.

Do NOT start implementing, and do not open a PR unless asked.
