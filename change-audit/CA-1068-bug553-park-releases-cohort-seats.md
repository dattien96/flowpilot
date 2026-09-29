# CA-1068 — BUG-553: stale waiting_user_approval children held cohort seats open forever

Date: 2026-09-29
Type: BugFix
Area: local-runner / park + cohort join

## Summary

`parkFlowForAwaitingUser` cancels in-flight child turns and stamps running
children `waiting_user_approval`, clearing dispatch/reinvoke intents — but
never buffered a terminal cohort result for the frozen members. The cohort
barrier only joins when buffered results reach the expected count, so
`hasOpenCohort` stayed true indefinitely and `flow-control("done")` /
`flow-control("continue")` soft-deferred forever (live run-15525: parked
children from an earlier escalate kept the cohort open permanently).

## Root cause

The Stop path (Task-241 B1) already buffers a `cancelled` cohort entry per
cancelled member so the barrier can join — the park path had no equivalent
seat release. A parked member was neither alive nor recorded dead: the
worst of both states.

## Fix — release parked members' cohort seats

- `releaseParkedCohortSeatsLocked(parentRunID)` iterates the parent's
  children, and for every non-terminal child with a `flowCohortId` buffers
  a `cancelled` cohort entry (same contract as Stop), then drains each
  cohort that reaches its expected count. Called from both
  `parkFlowForAwaitingUser` and `parkFlowForAwaitingUserLocked` before
  unlock. Repeated parks are idempotent — the duplicate-label dedup drops
  a second `cancelled` entry.
- `AgentOrchestrator.appendCohortResult`: a later real result for the same
  label **replaces** a `cancelled` placeholder instead of being deduped
  away. A parked member revived by a reprompt/resume intent (the CA-1062
  gated-child reprompt path) still counts toward the barrier; a drained
  key remains tombstoned (BUG-548) so dead-generation stragglers stay
  dropped.

## Files

- `apps/local-runner/internal/runner/interactive_service.go` —
  `releaseParkedCohortSeatsLocked` + calls from both park variants.
- `apps/local-runner/internal/runner/agent_orchestrator.go` —
  cancelled-placeholder replacement in `appendCohortResult`.

## Tests

`apps/local-runner/internal/runner/bug552553_park_decision_and_cohort_test.go`

- `TestBug553_ParkReleasesFrozenCohortSeats` — 2-member cohort, one
  completed + one live at park: `hasOpenCohort` becomes false.
- `TestBug553_StaleWaitingChildSeatAlsoReleased` — a child already parked
  `waiting_user_approval` by an earlier park (the live residue shape) gets
  its seat released on the next park.
- `TestBug553_RevivedMemberReplacesCancelledSeat` — a revived member's
  `completed` result replaces the `cancelled` placeholder and the barrier
  drains with the real verdict.

All verified RED on the pre-fix code and GREEN after.
