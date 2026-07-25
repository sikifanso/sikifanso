---
name: implement-design
description: Phase 2 of the issue workflow — implement and test an approved design spec from docs/specs/ for a GitHub issue. Use when the user says "implement issue N", "/implement-design N", or references an approved spec. Follows the spec; does not redesign.
---

# Implement a designed issue (Phase 2: build → test → record)

You are the **implementer**. The design work is done and lives in a spec under
`docs/specs/` — your job is to execute it faithfully, prove it works, and leave an
honest record. You were chosen for careful execution, not for redesigning; the spec
won an alternatives comparison you may not be able to see the full context of.

## Inputs

`$ARGUMENTS` is the issue number (e.g. `53`). If missing, ask for it.

## Step 1 — Locate and load the contract

1. Find the spec: `docs/specs/*issue-<N>-*.md` on the current branch. If it is not
   here, check the issue's comments — the design session posted the branch and path;
   fetch that branch and cherry-pick or merge the spec commit onto your branch.
2. Read the spec AND the issue (with comments — later comments may amend the design).
3. Check spec **Status**:
   - `Approved` → proceed.
   - `Draft` → ask the user for approval first. If the user already told you to
     implement in this conversation, that counts as approval: flip the header to
     `Approved` yourself and note it.
   - `Implemented` → stop and ask; this issue may already be done.

## Step 2 — Implement

- Follow the spec's **Implementation plan** in order. The spec's design decisions are
  settled — do not substitute your own approach because you'd have designed it
  differently.
- Deviation rules:
  - **Minor** (naming, mechanical detail, a helper the spec didn't foresee): do it,
    and record it in the spec's Deviations section (Step 4).
  - **Major** (a spec assumption turns out false, an approach doesn't compile/work,
    upstream behaves differently than researched): STOP. Do not improvise a new
    design. Report what broke, with evidence, and ask the user — the issue may need
    to go back to a design session.
- Keep commits scoped and messages descriptive; no signature lines (per CLAUDE.md).

## Step 3 — Test

- `make build && make test && make lint` must pass.
- Execute the spec's **Test plan**. Contract tests to keep in sync when surfaces
  change: `cmd/sikifanso/command_structure_test.go`, `internal/mcp/server_test.go`.
- Remote sessions cannot run Docker/k3d. For test-plan items marked as needing a real
  cluster, do NOT claim them verified — list them explicitly in your final report and
  in the spec as "manual verification pending", with the exact commands the user
  should run.

## Step 4 — Record and hand back

1. Update the spec in the same branch:
   - **Status:** `Implemented` (add the PR/branch reference once known)
   - Add a **Deviations** section (write "None." if none)
   - Annotate the Test plan with actual results, including what remains manual.
2. Update any CLAUDE.md/docs sections the spec flagged as becoming stale.
3. Commit and push to the session's designated branch. Open a PR only if the user
   asked for one; if you do, include `Closes #<N>` in the body.
4. Report to the user: what changed, test results (verbatim failures if any), and the
   manual verification checklist.
