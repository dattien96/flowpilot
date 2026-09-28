# BUG-540 — Completed provider turn never settles in-session; wedged hub recoverable only by restart

Status: **FIXED — regression tests green; live re-verify pending**
Severity: Critical-adjacent — a turn that is durably `terminal_completed` and
has `turn_completed` persisted in the event stream never runs its
settle/finalize tail in-session. `turnInFlight` stays held, every re-drive
(`maybeAutoReinvokeHub*`) is refused `turn_in_progress`, the flow wedges
silently, and only boot's `settle_owed` re-drive clears it.

## Symptom (live, `/tmp/fp-live4`, run-1663)

1. 00:03:10 — `agent-loop/continue` on a `member_stalled` loop reinvoked the
   arbiter hub turn `turn-6304` (devin).
2. 00:03:51 — provider finished (`agent_stopped cause:"complete"`,
   `stopReason:"end_turn"`), `turn_completed` persisted (event seq 597),
   dispatch ledger `terminal_completed` rev 4.
3. **No `[settle] finalized`, no gate eval, no verdict extraction, no
   follow-on turn — for ~37 minutes.** The arbiter's last_message ("re-check
   the current worktree state before submitting") never synthesized.
4. All watchdogs stayed silent: `turnInFlight` still held → hub reads `busy`
   → `hub_stalled`/`member_stalled` never refire. No card surfaced — the
   wedge is invisible to the operator.
5. A `memberAction: skip` on the stalled member appended the cohort result
   and cleared the loop block, but the follow-on
   `maybeAutoReinvokeHubWithNote` produced no turn — consistent with the
   re-drive being refused by the stuck `turnInFlight`.
6. Restart at 00:41:05: `[dispatch-settle] boot drive: 2
   terminal+settle_owed queued` → `[settle] finalized run=run-1663
   turn=turn-6304` — the boot re-drive ran the settle that never fired
   in-session. The other owed settle was `turn-4521` on run-2830, a reprompt
   turn from 23:10 — owed for ~90 minutes while three later turns ran fine.

## Root cause (inferred — needs confirmation by the fixer)

One of:

- The settle driver was never scheduled for these turns —
  `scheduleSettleDrive`/`DriveSettle` not invoked on the completion path for
  a reinvoke-dispatched hub turn (or a reprompt-dispatched child turn), OR
- The settle ran, hit a deferred branch (e.g. `pendingFlowGateSettle` →
  "gate still pending" at dispatch_settle_wire.go:79-81), retried via
  `RetrySettleWithBackoff`, exhausted silently (log-only), and never
  re-armed — leaving `settle_owed` durably but nothing to consume it
  in-session.

Either way the settle pipeline has no in-session catch-all: an owed settle
sits until process restart. The durable `terminal_completed` + missing
finalize is exactly the "explicitly-uncertain" state the three-outcome
contract is supposed to surface — instead it goes silent.

## Evidence

- Dispatch ledger `/tmp/fp-live4/.flowpilot/chats/fp-live4/dispatch.ndjson`:
  `turn-6304` prepared→send_claimed→send_started→terminal_completed (rev 4,
  seq 308-311); **no `gate_eval` effect** (turn-6561's settle, by contrast,
  carried the `gate_eval` effect at seq 316).
- Boot log: `[settle] finalized run=run-1663 turn=turn-6304` and
  `run=run-2830 turn=turn-4521` — both owed settles cleared only on restart.
- No `[settle] finalized` for either turn in the pre-restart log window
  (verified via grep).

## Why it matters

- A wedged hub turn silently freezes the whole flow: `turnInFlight` never
  clears → continues/reinvokes/member-action drives all refuse → the loop
  reads `running` but nothing can move. Operator sees a "running" run that
  is actually dead.
- Combined with BUG-539's reprompt drive-gap this produced a stable
  three-way deadlock live: arbiter unverdicted (this bug), child cycling
  reprompts (BUG-539), stall detectors shielded by "activity".

## Reproduction shape

Completed provider turn on a flow hub (or reprompted child) where the settle
tail is deferred/never scheduled → `turnInFlight` holds → subsequent drives
refuse → only boot `settle_owed` re-drive clears it.

## Expected fix shape (for the fixer)

- A periodic/idle sweep must consume owed settles in-session (the durable
  `settle_owed` flag exists — only boot reads it today).
- `RetrySettleWithBackoff` exhaustion must surface (card/event), not
  log-and-drop.
- A `turn_completed` event with no settle within a bound should trip the
  stall watchdogs as `explicitly-uncertain` rather than reading `busy`
  forever.

## Fix applied (CA-633)

- `dispatch_settle_wire.go`: boot settle walk extracted into `driveOwedSettles`
  — shared by `drivePendingSettlesOnBoot` and the new in-session sweep.
- `StartSettleSweep(ctx)`: 30s ticker (`settleSweepInterval`, mutable for
  tests) re-enumerates terminal `settle_owed` records and re-drives them
  in-session. Wired at server bootstrap next to `ScanDispatchRecoveryOnBoot`.
- `unstickSettleResidueLocked`: a settled turn whose gate eval exceeded
  `postTurnGateBusyBound` (or never ran: `gateCtxStart == 0`) releases the
  stale cancel/claim and `turnInFlight` before re-driving — ends the silent
  wedge where every watchdog read `busy` forever.
- `errSettleGatePending` sentinel: a genuinely pending gate defers without
  treating the record as exhausted; the deferred run re-drives
  `resumePendingFlowGate` which respects the blocked-loop contract.
- `surfaceSettleDriveExhausted`: `RetrySettleWithBackoff` exhaustion now
  stamps a durable `intentBlockedKind` diagnostic (operator-visible) instead
  of log-and-drop.

Tests: `bug540_settle_sweep_test.go` (3 tests — sweep settles a wedged owed
record, sweep unsticks a wedged gate eval and releases claims, live gate eval
left alone). All green.

## Live re-verification (2026-09-28, post-fix binary)

- Boot drive on :4322 (`fp-live5`): `[dispatch-settle] boot drive: 5
  terminal+settle_owed queued` → all five finalized — boot path intact.
- In-session settle verified on real flow children: tournament run-18354's
  launch turn (turn-18356) and child run-18660's turn (turn-18665) both ran
  `gate_eval → completion_event → graph_signal → dependents_release →
  finalizer` in-session seconds after terminal — previously this exact shape
  wedged 20–90 min until restart (live runs turn-4521/turn-6304).
- A slow-but-real gate eval (run-8002 `ping` turn, ~6 min `go test` gate on a
  large repo) held `gate_in_progress` while genuinely inside
  `postTurnGateBusyBound`, then completed normally — the sweep did not
  interfere with a live gate (matches `unstickSettleResidueLocked` live-gate
  guard + `errSettleGatePending` defer).
- Residual sibling finding (not settle_owed): run-18354 wedged at
  `waiting_review` via a deferred hub reinvoke — different channel, captured
  as BUG-542.
