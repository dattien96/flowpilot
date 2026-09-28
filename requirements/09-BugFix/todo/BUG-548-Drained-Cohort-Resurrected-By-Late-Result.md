# BUG-548 — drained cohort barrier resurrected by late member result / expected-count inference

## Status
FIXED — unit-verified (red→green) 2026-09-28. Live wedge reproduced on
`run-40950` (:4322 fp-live5) during BUG-539 skip-leg verification.

## Live sequence
1. `run-40950` tournament; `candidate-a` silent → `member_stalled` →
   `memberAction=skip` → step FAILED + cohort drained at 13:52.
2. The skipped member's late `TurnCompleted` (13:59) ran the normal
   completion path → `appendCohortResult` re-added an entry to the
   already-drained cohort → `ensureCohortExpectedLocked` re-inferred
   `expected=2`.
3. Buffer now held 1/2 for a cohort whose other seat was terminally
   skipped → `cohort_join_complete` unreachable → `hub_stalled`.

## Root cause
`drainCohort` deleted the live cohort maps but left no tombstone. Any
late result — or the sibling/count healer — could recreate state for the
drained key and reopen a barrier that had already settled.

## Fix (CA-641)
`cohortDrained map[string]bool` tombstone:
- `drainCohort` deletes live data AND sets the tombstone.
- `appendCohortResult` drops late results for drained keys.
- `inferCohortExpectedIfMissing` refuses to resurrect drained keys.
- `registerCohortMember` / `preRegisterCohort` clear the tombstone only
  for a genuinely new member generation (explicit re-registration), so a
  same-id rerun can still open a fresh cohort.

## Tests
`internal/runner/bug548_cohort_drain_tombstone_test.go` (red→green):
drain → late append + inference → buffer stays 0 / no expected
re-registered; explicit re-registration clears the tombstone.

## Notes
Same family as BUG-546 (seat arithmetic vs physical rows): both stem
from cohort state being re-derivable after it should be final.
