# CA-633 — In-Session Settle Sweep (BUG-540)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-540

---

## Problem

`drivePendingSettlesOnBoot` was the only enumeration of terminal
`settle_owed` records. In-session, a completed turn whose settle tail was
deferred/never scheduled left `turnInFlight` pinned forever — every
watchdog read `busy`, subsequent drives refused `turn_in_progress`, and
only a restart cleared it (live: turns wedged 20–90min, cleared at boot).
`RetrySettleWithBackoff` exhaustion was log-and-drop.

## Fix

### `internal/runner/dispatch_settle_wire.go`

- Boot walk extracted to `driveOwedSettles` — shared by boot recovery and
  the new `StartSettleSweep(ctx)` ticker (`settleSweepInterval`, 30s,
  mutable for tests), wired at server bootstrap next to
  `ScanDispatchRecoveryOnBoot`.
- `unstickSettleResidueLocked` — a settled turn whose gate eval exceeded
  `postTurnGateBusyBound` or never started releases stale cancel/claim and
  `turnInFlight` before re-drive.
- `errSettleGatePending` sentinel — a genuinely pending gate defers
  (record stays owed; `resumePendingFlowGate` re-drives per the blocked-
  loop contract) instead of being consumed as a failure.
- `surfaceSettleDriveExhausted` — retry exhaustion stamps a durable
  `intentBlockedKind` diagnostic (operator-visible) instead of
  log-and-drop.

## Tests

`bug540_settle_sweep_test.go` (3 tests) — sweep settles a wedged owed
record; sweep unsticks a wedged gate eval and releases claims; a live
gate eval inside the busy bound is left alone. All green.
