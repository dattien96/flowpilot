# CA-645 — Cohort-Less Member Respawn Rebinds the Open Barrier Seat (BUG-1194)

**Date**: 2026-10-05
**Author**: Devin
**Ticket**: BUG-1194

---

## Problem

Live `run-225691` (PrivateVault Task-0310 sprint): `owner_1` leg
`run-258141` finished its turn but its settle was consumed by a
debate-mount divert, so no cohort entry was appended — the
`flow-auto-debate_trigger-round-4` barrier sat at 1/2 waiting on a seat
labelled `owner_1`. The stall sweep parked the hub (`member_stalled`),
the Continue path re-dispatched the member, and the replacement leg
`run-260972` was spawned by the internal respawn in
`resumeFlowWithFeedback` — a `SpawnAgentInput` carrying **no**
`FlowCohortID`, no `CohortSize`, no `CohortSeatInherited`
(`child_spawn_requested` diag showed `flow_cohort_id: ""`,
`auto_orchestrate: true`).

That replacement ran, settled cleanly, and its completion was a no-op for
the barrier: `settleFlowChildTurnCompletedLocked` keys the cohort join off
`flowCohortId`, and the leg had none. The barrier stayed 1/2, owner_1
stayed RUNNING, and the member_stalled → respawn → drop loop repeated
every sweep round until manual intervention.

Root cause: `spawnChildRun` took `in.FlowCohortID` verbatim. Every
cohort-less spawn — the member_stalled Continue respawn, the
failed-delegate fresh spawn, a hub ad-hoc `spawn_agent` (the hub cannot
know the engine's round-scoped cohort id) — produced a leg that could
never fill the seat an open barrier was holding for its label.

## Fix

`spawnChildRun` now resolves an open seat before cohort registration:
when `in.FlowCohortID` is empty and the spawn did not already inherit a
seat, it looks up `openCohortSeatForLabelLocked(parentRunID, seatLabel)`
where `seatLabel` is `in.Label` falling back to `in.Agent` — the same
effective-label fallback stored on `rs.label`. A hit sets
`rs.flowCohortId` and `in.CohortSeatInherited = true`, so the barrier's
expected count is **not** grown — the seat was already counted when the
original member was registered. A `child_spawn_cohort_rebound` diag entry
records the rebind.

`openCohortSeatForLabelLocked` (new, `cohort_stall.go`) scans the parent's
children newest-first for a prior leg carrying the same effective label,
then asks `openSeatForLabel` whether that leg's cohort is still waiting on
the label: barrier open (`expected>0`, `buffered<expected`) and no real
entry already consumed the label's seat (`cancelled` placeholder entries
release the seat per BUG-553). A drained barrier or an already-buffered
label returns `""` — the fix never re-opens a delivered cohort and never
double-binds a consumed seat. Explicit re-drives keep owning drained-cohort
rejoin via `rearmCohortIfDrainedLocked`.

This mirrors the already-correct provider-switch respawn in
`quota_gate.go` (`FlowCohortID: child.flowCohortId` +
`CohortSeatInherited: !memberAlreadyBuffered(...)`) and the reused-child
re-tag in `reinvokeExistingFlowChild`.

## Files

- `internal/runner/interactive_service.go` — seat rebind inside
  `spawnChildRun` before cohort registration.
- `internal/runner/cohort_stall.go` — `openSeatForLabel` +
  `openCohortSeatForLabelLocked`.
- `internal/runner/bug1194_respawn_rebinds_cohort_seat_test.go` —
  red→green tests.

## Test evidence

- `TestBug1194_RespawnRebindsOpenCohortSeat` — red before the fix (spawn
  landed with `flowCohortId=""`), green after: replacement binds the open
  seat, expected stays 2, its completion drains the barrier.
- `TestBug1194_RespawnAgentNameFallbackBindsSeat` — label-less spawn falls
  back to the agent name for seat matching; expected stays 2.
- `TestBug1194_SpawnWithoutHeldSeatStaysUnbound` — a consumed seat does
  not rebind (second entry would dedupe away — binding would be a lie).
- `TestBug1194_UnrelatedLabelSpawnStaysUnbound` — a spawn whose label no
  prior leg carried stays cohort-less.

Provider parity: provider-agnostic — the rebind runs before provider
selection and only touches orchestrator/durable-run fields.
