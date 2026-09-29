# BUG-551 — resume-path quiet wedge: escalate park + cap-blocked reinvoke leaves loop "running" with no turn

## Status
RESOLVED — CA-1066. All three seams + the adjacent cap-blocked watchdog
hole are fixed; 4 regression tests in
`internal/runner/bug551_quiet_wedge_recovery_test.go` (RED→GREEN
verified).

## Live-found during
Post-fix live verification on `:4421` (runner v3, `/tmp/fp-vibe-item2`
store root), 2026-09-29 ~21:29–21:44.

## Live sequence
1. Sprint `synthesis` turn escalated → `flow_parked_awaiting_user`
   cancelled in-flight turns and dropped auto-intents.
2. Operator `continue` → loop unblocked → `hub_reinvoke_scheduled` →
   immediately **`hub reinvoke blocked by round cap`** (round 4 / cap 5).
   The reinvoke was dropped; `synthesis` stayed stamped RUNNING; run
   snapshot stayed `running`.
3. `extend-cap` (cap→7) + `continue` → **nothing re-drove**:
   - `resumeFlowWithFeedback` early-returns when `!wasBlocked` (loop
     already `running`) — the reinvoke tail at the escalate path is never
     reached.
   - `resumePendingLoopWork` → `redriveQuietFlowLoop` — quiet check fails
     because `pendingAgentContext` is non-empty (the joined result note
     the cancelled turn never drained).
   - The hub-stall watchdog is in-memory and was never armed inside the
     restarted process, so `hub_stalled` never re-fires to re-block the
     loop.
4. Result: durable "quiet wedge" — loop `running`, zero in-flight work,
   zero pending intents, all operator surfaces (continue / feedback /
   gate-decision / admin approvals) inert. Only recovered by POSTing a
   manual turn onto the hub run, after which the flow settled
   `done + plan_complete` — proving the residue, not the flow, was the
   blocker.

## Root cause
Three independent recovery holes line up:

- **Early-return on unblocked loop**: `resumeFlowWithFeedback` treats
  `st.Status != "blocked"` as "nothing to do" — but a running loop can be
  dead (cap-blocked reinvoke, dropped intent, crashed turn). The quiet
  check belongs on this path too, not only on `resumePendingLoopWork`.
- **`pendingAgentContext` misclassified**: `redriveQuietFlowLoop`'s quiet
  predicate treats a non-empty context list as "a turn is coming to drain
  it". When the owning turn was cancelled by the escalate freeze, the
  note is orphaned — it is exactly what a redriven hub turn must
  consume, not a reason to stay quiet.
- **Watchdog not re-armed on rehydrate**: `hubStallTimers` is in-memory;
  after a restart a run whose loop is `running` with a stale RUNNING
  step stamp and no turn never gets the stall check scheduled — the
  recovery path that would re-block (and thereby unblock via continue)
  never exists.

## Fix direction (not implemented)
- On a continue where the loop is already `running` but the run is quiet
  (no in-flight turn, no armed intent, stale RUNNING step), fall through
  to the quiet-redrive path instead of early-returning.
- In `redriveQuietFlowLoop`, treat orphaned `pendingAgentContext` as a
  reason TO redrive the hub (the turn must drain it) — distinguish
  "context awaiting an already-scheduled turn" from "context orphaned by
  a cancelled turn".
- Re-arm the hub-stall watchdog on session rehydrate when the loop is
  `running` and no turn is in flight, so a wedged loop re-surfaces as an
  actionable `hub_stalled` card.

## Reproduction
1. Flow run parked on `flow_parked_awaiting_user` with a RUNNING hub
   node stamp.
2. Continue while `round >= effectiveCap` → reinvoke cap-blocked.
3. `extend-cap`, restart the runner process (or let the wedge occur
   in-process), then `continue` — loop reports `running`, no turn ever
   dispatches.

## Regression test shape (when fixed)
- Unit: parked-unblock → cap-blocked reinvoke → extend-cap + continue →
  hub reinvoke is re-armed/dispatched (red→green on the quiet-wedge
  seam).
- Unit: `redriveQuietFlowLoop` with non-empty `pendingAgentContext` and
  no in-flight turn still re-drives the hub.
- Unit: rehydrated `running` run with stale RUNNING step and no turn
  gets the stall watchdog scheduled.
