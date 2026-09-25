---
id: CA-951
title: Resolve CA-ID collision CA-916..938 after rebase — branch-side entries renamed with `b` suffix
type: Docs
feature: dev-infra
date: 2026-09-24
status: done
---

## Context

After rebasing `cp_live_test` onto `origin/main` (CP-81..84 merge), the
change-audit ledger contained **23 colliding IDs**: both sides independently
minted `CA-916` through `CA-938` with different content.

- Main side: CP-81/82/83/84 + Desktop work (`CA-916-Scaffold-Live-Progress-*`,
  `CA-920-Desktop-Lease-Register-Retry`, `CA-936-JSONRPC-Error-Data-Detail`, …)
- Branch side: bug-wave clusters A–V (`CA-916-Devin-ACP-Provider-Fixes-*`,
  `CA-920-Flow-Gate-Enforcement-*`, `CA-936-resume-stamp-child-reprompt-*`, …)

Filenames stayed unique, but every bare `CA-9NN` citation in docs became
ambiguous (e.g. `BUG-454` cites `CA-936` meaning the branch entry, while
`BUG-278`/`CP-41` cite `CA-938` meaning a historical sandbox ledger).

## Resolution

Renamed all 23 branch-side files to the repo's existing collision precedent
(`CA-689b-…`): `CA-NNN-<branch-title>.md` → `CA-NNNb-<branch-title>.md`, and
updated `id:` frontmatter plus cross-references inside them.

Reference rewrite rule: `CA-NNN\b` → `CA-NNNb` applied only to **branch-authored
documents** (docs whose refs do not exist in the `origin/main` version of the
file):

- `requirements/09-BugFix/done/BUG-363`, `BUG-374`–`BUG-454` docs
- `requirements/07-Coding-Plan/done/CP-Test-Progress-Tracking.md`
- the renamed CA files themselves (self `id:` + cross-refs)

Deliberately NOT touched:

- Filename-shaped refs (`CA-NNN-<slug>.md`) — e.g. `CA-922-snake-mvp.md`,
  `CA-922-calc-core-lcm.md`, `CA-924-calc-core-manual-sum-not-reproducible.md`
  are audit files minted by live runs inside sandbox beds, not ledger refs.
- Refs already present in `origin/main` versions of docs (`Task-4NN`,
  `CP-82/83/84`, `CP-41/43/51/58/62/64` test-steps, `BUG-278`) — these point to
  main-side CAs or historical sandbox ledgers.
- Code comments — verified none carry branch-only `CA-916..938` refs.
- Commit messages on the rewritten branch — immutable history; the `b`-suffixed
  names are the canonical post-rebase identifiers.
- `CA-002` and `CA-689/CA-689b` — pre-existing duplicates predating the rebase.

## Verification

- `rg --pcre2 'CA-9(1[6-9]|2[0-9]|3[0-8])\b(?!-)'` over all branch-authored
  docs returns zero hits; every ambiguous bare ref now reads `CA-NNNb`.
- Main-side refs in main-authored docs verified unchanged
  (`git show origin/main:<file>` comparison).
