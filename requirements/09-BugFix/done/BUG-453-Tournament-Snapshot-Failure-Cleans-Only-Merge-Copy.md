---
id: BUG-453
title: Tournament silently drops patch snapshot then deletes its worktree on escalation
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-414, CA-923]
---

## AI Quick View
- **What**: A human card can offer an unmergeable candidate after a patch-snapshot error.
- **Why**: Arbiter ignores Diff errors, then escalation cleanup deletes all candidate worktrees.
- **Key constraint**: Preserve a recoverable copy of every selectable candidate before cleanup, or fail closed with an explicit recovery action.

## 1. Metadata
- Document ID: `BUG-453`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `tournament-harness`
- Parent Documents: [BUG-414](../done/BUG-414-TieCard-Resolution-Discarded.md), [CA-923](../../../change-audit/CA-923-tournament-escalation-dispatch-card-and-join-fixes.md)

## 2. Symptom and Impact
`behaviorTournamentArbiter` snapshots each candidate patch only on successful `mgr.Diff`, silently ignoring a non-nil error (`internal/runner/tournament_behavior.go:354-361`). For escalate verdict, `finishTournamentArbiter` always calls `clean(ids)` **before** returning the optional `patches` payload (`:363-369,410-415`). If Diff failed for a candidate (e.g. unreadable git state), the decision card can still offer it, but its worktree is gone and no stored patch exists. `behaviorTournamentMerge` falls back to `mgr.MergeWinner` when `patch` is empty (`:436-456`), which returns 'no worktree for owner' — the exact BUG-414 incident this change purported to eliminate. An empty successful diff has the same fallback when no worktree survives. Severity: **high** for candidate data loss / unresolvable human choice.

## 3. Reproduction / Evidence
Inject `mgr.Diff` error (or empty diff) for one candidate while arbiter chooses `escalate`; inspect returned payload and clean callback, then submit that candidate as the card choice. Static, ordered code-path proof; no live tournament run or fault-injection test performed in this review. Existing BUG-414 tests cover stored nonempty patches, not snapshot failures.

## 4. Acceptance and Verification
Add assertion-first error/empty snapshot E2E test, verify any card option has a durable merge source after cleanup. On snapshot error fail closed or retain worktree and surface explicit status; preserve legitimate empty-patch behavior. Test crash/restart and merge conflict paths without weakening existing tests; live verify before marking BUG-414 complete.

## 5. Resolution (2026-09-23, CA-931)

- Arbiter fails closed on snapshot/diff acquisition errors; `winnerPatch`
  lookup is presence-aware so an absent entry is not mistaken for a valid
  empty patch — patch loss surfaces instead of merging an empty diff.
- Tests: `bug446_453_tournament_test.go` — snapshot-failure merge-source
  preservation + absent-vs-empty patch cases; red before fix.
