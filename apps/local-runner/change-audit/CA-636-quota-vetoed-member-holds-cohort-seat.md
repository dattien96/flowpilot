# CA-636 — Quota-Vetoed Cohort Member Holds Its Barrier Seat (BUG-544)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-544

---

## Problem

Live `run-25217`/`run-26015`: a spawned tournament candidate whose first
turn vetoes at the quota gate was routed through
`handleChildStartTurnFailure` — stamped FAILED, appended a `failed` cohort
entry, leg closed — while its `quota_route_required` card sat pending.
The barrier counted the member immediately: sibling completion satisfied
`expected`, the join fired, the arbiter ran, the flow completed, and the
loser sweep deleted the vetoed member's worktree. Every later card answer
failed `worktree ... no longer exists (swept)` — the pending card became
permanently unresolvable.

## Fix

### `internal/runner/interactive_service.go`

- `handleSpawnedChildTurnFailure`: `quota_route_required` returns without
  touching the member — the durable card is the intent. No FAILED stamp,
  no cohort append, no leg close. New diag event
  `spawned_child_quota_parked`.

### `internal/runner/agent_orchestrator.go`

- `SpawnAgentInput.CohortSeatInherited`: spawn takes over a held barrier
  seat without bumping `cohortExpected` (flowCohortId still tags the child
  so its completion joins the right barrier).

### `internal/runner/quota_gate.go`

- `respawnChildOnRoute`: sets `CohortSeatInherited` when the vetoed member
  never buffered an entry (first-dispatch veto). A member that already
  appended `failed` (mid-turn veto at TurnFailed) consumed its seat — the
  successor registers a fresh one, preserving the grown expected count.
- `applyQuotaRouteAnswer` `stop` branch: appends the `failed` cohort entry
  the join waits on (deduped via `memberAlreadyBuffered`) and closes the
  member's leg claim durably — without this the barrier would wait on a
  member nobody will ever run.

## Invariants preserved

- Barrier still bounded: card `stop` / expiry→member-stall / skip all
  produce a terminal member entry; nothing waits forever.
- `memberAlreadyBuffered` dedupe keeps a double-append impossible.
- Spawn-first ordering (BUG-534) and the `liveClaim` worktree guard
  (BUG-535) unchanged.

## Verification

- Red→green: `bug544_quota_veto_cohort_seat_test.go` (3 tests).
- Focused quota/cohort/spawn batch green; only TempDir cleanup noise.
- Live: `run-25217` reproduced the defect; post-fix tournament
  `run-26036` exercised veto→hold→answer→respawn (CP-Full-Live-Test R13).
