# BUG-546 — cohort expected-count inflated by physical runs after quota-veto successor

## Status
FIXED — unit-verified (red→green), live leg verified 2026-09-28
(run-37723 on :4322 fp-live5, mid-cohort restart).

## Live-found during
`run-34296` (tournament-harness on :4322), 2026-09-28 — the residual wedge
observed *after* the BUG-544 fix landed.

## Live sequence
1. Candidate-a `run-34317` vetoed on grok → `spawned_child_quota_parked`
   (BUG-544 fix working: parked, seat held, no terminal entry).
2. Route answer → successor `run-34359` spawned with `CohortSeatInherited`
   → expected stayed 2. Both candidates reached DONE.
3. Runner restarted mid-cohort (BUG-545 rebuild, ~12:39).
4. Resume → `reconstructPendingChildSessions` rebuilt the cohort from
   **session rows**: `preRegisterCohort(cid, len(members))` counted
   34317 + 34359 + 34322 = **3** for 2 logical seats.
5. The buffer dedups by label (`appendCohortResult`), so candidate-a can
   only occupy one entry — `cohortComplete` (len(buffer)>=expected)
   unreachable at 2/3 → `cohort_join_complete` never fired → `hub_stalled`.

`ensureCohortExpectedLocked` (the live-sibling healer for run-98153) had
the same run-counting shape.

## Root cause
Expected-count inference/rebuild counted physical run/session rows.
BUG-544 made "one logical seat = possibly two physical runs" a real
configuration (vetoed leg + successor share `label`); every inference
site must now count **seats**, not rows.

## Fix (CA-639)
Both inference sites count distinct seats keyed by the same label the
buffer dedups on:
- `ensureCohortExpectedLocked`: sibling scan dedups `r.label` (run-id
  fallback for unlabeled members).
- `reconstructPendingChildSessions`: seat set via
  `firstNonEmptyResumeValue(child.label, session.Label, session.AgentName)`,
  run-id fallback — matching the buffer append's label resolution.

## Tests
`internal/runner/bug546_cohort_seat_dedup_test.go`:
- two runs labeled `candidate-a` + one `candidate-b` → inferred 2;
  buffer of both labels → `cohortComplete` true;
- three unlabeled members → per-run counting preserved (3).

## Live verification
Fresh tournament `run-37723` on the fixed binary: candidate-a `run-39034`
vetoed → parked → route answer → successor `run-39327`; candidate-b
`run-39039` completed. Server restarted mid-cohort; resume ran the
seat-deduped rebuild → `expected=2`, both seats buffered →
`joinRecoveredCohort` drained and emitted the cohort note
(`hub_reinvoke_deferred` with `cohort_note_len=459` at 13:08:31Z;
step stamps `candidate-a→CANCELED`, `candidate-b→DONE`). On the pre-fix
binary the identical restart left `expected=3` and wedged.

Residual (separate concern, not the barrier bug): the root normalized to
`cancelled` on resume — a mid-turn kill leaves no pending-gate child, so
the recovered cohort note defers instead of driving the arbiter. Same
class as the earlier nested-tournament post-restart limitation.
