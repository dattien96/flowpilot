# CA-1116 — BUG-580: review verdict survival across cohort joins and drained-cohort re-completions

## Why

Live run-100368 (PrivateVault Task-025): the synthesis gate read the
reviewer verdict as missing forever even though reviewers submitted
`approved` three times. Two stacked defects ate every copy:

1. `snapshotReviewCohortVerdictsLocked` REPLACED
   `lastReviewCohortVerdicts` on every cohort join, so a later
   owner-debate join erased the reviewer's recorded verdict.
2. `appendCohortResult` correctly drops a member re-completion that lands
   on a drained (already-delivered) cohort barrier — but the verdict had
   already been consumed out of `pendingReviewVerdictByLabel` during
   settle, so it vanished from BOTH places and nothing surfaced it to the
   hub gate.

## What changed

`apps/local-runner/internal/runner/review_done_verdict.go`:

- `snapshotReviewCohortVerdictsLocked` now merges per-label instead of
  replacing the map: a member's fresh verdict overwrites only its own
  label; a member re-joining verdict-less clears its own stale entry (a
  superseded approval must not satisfy the gate for a round that
  produced nothing); labels absent from this join are left untouched.

`apps/local-runner/internal/runner/agent_orchestrator.go`:

- `appendCohortResult` returns `dropped bool` reporting that the entry
  hit a drained cohort tombstone. Barrier semantics are unchanged —
  drained cohorts still refuse late entries so the barrier cannot
  re-open (BUG-548).

`apps/local-runner/internal/runner/interactive_service.go`:

- `settleFlowChildTurnCompletedLocked`: when the cohort append is
  dropped-on-drained and the entry carried a machine verdict, the verdict
  (and detail) is restored to `pendingReviewVerdictByLabel` —
  `mergePendingReviewVerdictsLocked` surfaces it on the next gate read.
  Diag event `cohort_verdict_restored_after_drain` records the rescue.

## Invariant

A recorded machine verdict is durable until consumed by a gate decision.
Barrier tombstones protect cohort delivery ordering; they must never be
a verdict sink.

## Tests

`bug580_review_verdict_survival_test.go` (red → green):

- a later non-review cohort join preserves unrelated verdicts
- a verdict-less member re-join clears only its own stale verdict
- a reviewer re-completion into a drained cohort keeps its verdict in
  the pending map for the gate
