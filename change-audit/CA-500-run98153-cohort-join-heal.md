---
id: CA-500
feature_key: agent-flow-engine
title: Heal lost cohortExpected so dual-reviewer join fires
date: 2026-08-14
status: COMPLETE
---

## Change

run-98153 (same residual as CA-467 / run-91842): both `grok-review` and
`my-reviewer` completed and were buffered (`cohort_member_completed`) but
`cohort_join_complete` never logged. `cohortComplete` requires
`expected > 0`; when RAM `cohortExpected` is 0 the barrier stays open
forever and `grok-synthesis` never starts. Desktop uses the same runner.

Before `cohortComplete` on member settle / fail / stop, restore expected
from live siblings with the same `flowCohortId` **only when expected is
already 0**. Does not shrink a pre-declared `CohortSize` (3-reviewer
still waits for 3).

Join from `settleFlowChildTurnCompletedLocked` already holds `s.mu`.
Calling `snapshotReviewCohortVerdicts` (which locked `s.mu` again)
deadlocked once heal made join fire on that path. Settle now uses
`snapshotReviewCohortVerdictsLocked`. Public wrapper unchanged for
existing callers.

Does not undo BUG-318 re-tag, BUG-307 sealed-loop drain, BUG-234
self-settle, or CA-467 TUI work.

## Provider impact

Case 1 (agnostic). `settleFlowChildTurnCompletedLocked` / orchestrator
maps take no `providerKey`. New tests matrix Claude / Codex / Grok.

## Tests

New `run98153_dual_reviewer_cohort_join_test.go` only. Old suite
untouched.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-36
change_type: bugfix
summary: Infer lost cohortExpected from live siblings so 2/2 reviewer join still fires
# --->8---
