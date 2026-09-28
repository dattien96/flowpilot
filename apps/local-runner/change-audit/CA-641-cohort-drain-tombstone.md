# CA-641 — Drained Cohort Tombstoned Against Late Straggler Appends (BUG-548)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-548

---

## Problem

Live `run-40950` (`:4322`, stall-timeout test bed): both tournament
candidates were skipped via `memberAction=skip` after `member_stalled`
fired. The skip path appended a `failed` entry per seat, the barrier
completed, and `drainCohort` delivered + cleared the cohort at 13:52:47.

At 13:59:02 the route successor `run-42814` (spawned before the skip, still
mid-turn) finished and its completion path ran `ensureCohortExpectedLocked`
→ `appendCohortResult`. The expected count was 0 after the drain, so the
heal **re-inferred** `expected=2` from live sibling seats
(`cohort_expected_inferred`), then the late result appended `candidate-a`.
Buffer sat at 1/2 forever — `hasOpenCohort` stayed true, the hub parked on
`hub_stalled` at 14:00:03.

Root cause: `drainCohort` deleted the buffer and expected count but left no
record that the cohort had already delivered. A late straggler append +
the RAM-loss heal then rebuilt a barrier that can never complete (the other
seats' entries were already consumed and dedup-by-label means they will
never re-append).

## Fix

`AgentOrchestrator` gained a `cohortDrained map[string]bool` tombstone:

- `drainCohort` sets the tombstone after extracting entries.
- `appendCohortResult` drops appends to tombstoned keys
  (`drop-drained` diag) — a delivered cohort does not accept stragglers.
- `inferCohortExpectedIfMissing` no-ops on tombstoned keys — the heal path
  cannot resurrect a delivered barrier.
- `registerCohortMember` / `preRegisterCohort` clear the tombstone — a NEW
  member generation that legitimately reuses the same cohort key
  (e.g. a fresh `parallel_rollout-attempt-N` round or the post-restart
  rebuild) still works normally: registration re-opens the key, inference
  fills expected, appends buffer.

The tombstone is in-memory like the rest of the barrier state — post-restart
rebuilds start from durable sessions and are unaffected (the `cohortNeed`
gate already skips fully-terminal cohorts).

## Files

- `internal/runner/agent_orchestrator.go` — `cohortDrained` map + guards in
  `drainCohort`, `appendCohortResult`, `inferCohortExpectedIfMissing`,
  `registerCohortMember`, `preRegisterCohort`.
- `internal/runner/bug548_cohort_drain_tombstone_test.go` — red→green tests.

## Test evidence

- `TestBug548_LateAppendAfterDrainDoesNotReopenBarrier` — red before the fix
  (`hasOpenCohort` true after drain → infer → late append); green after:
  inference skipped, append dropped, expected stays 0.
- `TestBug548_NewGenerationRegistrationClearsTombstone` — a re-registered
  cohort key joins normally (reuse not broken).
- Live trigger sequence on `run-40950`: skip ×2 → drain → successor
  `run-42814` late `cohort_member_completed` → `cohort_expected_inferred`
  → `hub_stalled` — the exact wedge this CA prevents.
- Focused: `go test -count=1 ./internal/runner -run 'TestBug548|TestBug546|TestBug544|TestBug547'` — green.

## Risk / blast radius

- Only the orchestrator barrier maps are touched; all four mutation sites
  are covered. No wire/API surface changes.
- `handleMemberAction` skip, normal `cohort_join_complete`, and
  `joinRecoveredCohort` all funnel through the same drain — uniformly
  protected.
