# CA-1123 — BUG-571: armed pendingFlowGateSettle with no driver wedges forever

## Why

Live run-100368 (leg run-124283 / turn-1252xx, parent turn-126711):
`pendingFlowGateSettle` stayed armed on both a child leg and the parent run
with no live gate evaluation and no recoverable settle record. The only two
drivers that evaluate the armed flag — `drivePendingSettlesOnBoot` (restart)
and `driveOwedSettles`/`driveOwedSettleRecords` (in-session sweep) — both
enumerate terminal dispatch records with `SettleOwed`. When the flag was
armed without such a record (or the record shape fell outside
`ListRecoverable`), nothing ever called `resumePendingFlowGate`; only an
operator `agent-loop/continue` un-wedged it.

## What changed

`apps/local-runner/internal/runner/wedge_sweep.go` (new):

- `sweepWedgedFlowWork` scans live `s.runs` for
  `pendingFlowGateSettle == true` with no gate claim and no live post-turn
  gate eval (`gateCancelLive` false), and re-drives
  `resumePendingFlowGate` — the existing single-flight, fail-closed owner of
  the disposition.

`apps/local-runner/internal/runner/dispatch_settle_wire.go`:

- `StartSettleSweep` now also invokes `sweepWedgedFlowWork` on each tick —
  the settle sweep is the only in-session watchdog, so armed-but-unlisted
  state converges through it.

## Invariant

An armed `pendingFlowGateSettle` must always have a driver. The sweep never
evaluates a gate that a live post-turn eval or claim already owns
(single-flight preserved), and `resumePendingFlowGate` retains every
fail-closed check (stop-generation fence, blocked parent loop, gate claim).

## Tests

`bug571_wedge_sweep_test.go` (red → green):

- `TestBug571_SweepConvergesArmedGateSettle` — armed settle with no
  claim/cancel is re-driven and clears.
- `TestBug571_SweepLeavesLiveGateEvalAlone` — armed settle with a live
  `postTurnGateCancel` + claim inside the busy bound is untouched.
