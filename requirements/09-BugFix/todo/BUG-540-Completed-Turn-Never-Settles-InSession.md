# BUG-540 — Completed provider turn never settles in-session; wedged hub recoverable only by restart

Status: **CAPTURED — live reproduction, not yet fixed**
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
