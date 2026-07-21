# CA-391 — run-33289 post-Stop follow-up hang (dispatch stop fence)

## Summary

Live Grok Review Loop **run-33289**: coder completed → reviewers started → user
**Stop** → chat follow-up `"done hay bị cancel rồi ?"` **hung** (Thinking,
no answer). Turn log had the prompt (`turn-33758`); `last_message` empty.

## Root cause (not Grok-specific)

1. **BUG-308** correctly **admitted** the follow-up (`loop_status=stopped` no
   longer 409s). Evidence: turn log prompt + dispatch `prepared`/`send_claimed`.
2. **Stop** also set durable dispatch **`run_stop.stopped=true`** (generation=1)
   and `status=cancelled`.
3. `linearizeSendStarted` hit **`ErrRunStopFence`** →
   `terminal_cancelled` **without** `EventTurnFailed`/`EventTurnCompleted` →
   desktop hang after optimistic `TurnStarted`.
4. Stranded entry-spawn note remained in `pending_agent_context` on disk (snap
   before drain); BUG-307 drain would still strip it for a live send, but the
   send never happened because of the fence.

Claude “worked” earlier for **loop done** follow-ups (BUG-302/305) because
**done does not set the dispatch stop fence**. This path is **Stop-only** and
is **Case 1 provider-agnostic** (shared startTurn/runTurn + DispatchStore).

## Fix

- On sealed-loop follow-up admission: revive `status` from `cancelled` →
  `running`.
- Before linearize: `ReleaseRunStopFence` clears **Stopped** only (keep
  **Generation** so children stay fenced).
- `abortDurableStartIfStaleLocked` skips sealed plain-chat follow-ups.
- Defense-in-depth: stop-fence cancel path emits **EventTurnFailed** so UI
  never hangs if release fails.

## Cross-provider parity

Case 1 — no `providerKey` branch. New test table: **Codex + Grok**.

## additive-tests-only

New file only:
`bug308_stop_fence_followup_hang_test.go`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-308
change_type: bugfix
summary: After user Stop, plain-chat follow-ups release the hub dispatch stop fence (keep generation) and revive cancelled status so send_started can proceed; stop-fence cancel emits TurnFailed so the UI does not hang (run-33289).
# --->8---
