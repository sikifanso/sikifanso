# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the
codebase.

This is a **single-context** repo: one `CONTEXT.md` at the root, one `docs/adr/` directory.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root — the ubiquitous language of this repo. It exists and is
  actively maintained.
- **`docs/adr/`** — read ADRs that touch the area you're about to work in. This directory does
  not exist yet; `/domain-modeling` creates it lazily when the first hard-to-reverse decision
  is recorded.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't
suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs`
and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get
resolved.

## File structure

```
/
├── CONTEXT.md
├── docs/adr/          ← created lazily by /domain-modeling
└── internal/, cmd/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis,
a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary
explicitly avoids — the glossary records an `_Avoid_:` list under each term for exactly this.

For example, `CONTEXT.md` distinguishes **held**, **derived**, **pinned**, and
**version-locked** in the dependency-governance area; these are not interchangeable, and
"blocked"/"frozen"/"capped" are explicitly avoided.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing
language the project doesn't use (reconsider) or there's a real gap (note it for
`/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently
overriding:

> _Contradicts ADR-0007 (event-sourced orders) — but worth reopening because…_
