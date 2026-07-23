# CA-414: blocked restart preserves Continue/Stop (run-63960)

## Symptom

Live Review Loop `run-63960` parked at cap (`loopState.status=blocked`,
`blockReason=cap`). After runner restart:

1. Opening the chat / freeform prompt showed **Failed**
2. **Continue / Stop** card was missing
3. Engine risked re-running post-turn gate → gate reprompt → `startTurn` →
   `flow_awaiting_user` 409

## Cause (two layers)

### Runner

On reconstruct, durable `PendingFlowGateSettle` + gate-reprompt remained true
while `LoopState.Status=blocked`. Boot scheduled `resumePendingFlowGate`, which
re-evaluated the gate and tried a reprompt turn; `startTurn` correctly rejected
with `flow_awaiting_user`, but the auto path left the UX as a failed chat turn
and could race durable session writes.

### Desktop

1. `sendPrompt` mapped every send error to `status=failed` (including 409
   `flow_awaiting_user`), which is not a hard failure — the flow is parked for
   human decision.
2. `openHistoryRun` cleared `agentGraphSnapshot` and only restored graph via
   SSE later. `FlowAwaitingUserCard` requires
   `agentGraphSnapshot.loopState.status === "blocked"`, so Continue/Stop was
   invisible until (if ever) SSE arrived.
3. Timeline settle only ran for `loopState.done`, leaving stale Thinking rows
   at cap park (CA-413).

## Fix

### Runner (`interactive_resume.go`, `interactive_service.go`)

- Restore `LoopState` whenever durable `Status` is set (not only mode/cap/round).
- On reconstruct when `Status=blocked`: clear stale settle + gate-reprompt
  **payload** via `clearStaleFlowGateIntentsLocked` (shared helper).
- **Preserve `pendingGateRepromptGen` high-water** (run-23820 invariant) —
  never set gen to 0.
- Snapshot under `s.mu`, then persist cleared intents + LoopState.
- `resumePendingFlowGate`: early-return for `stopped` | `done` | `blocked`,
  and durably clear stale settle for all three (not only blocked).

### Desktop (`store.ts`)

- `sendPrompt`: map `flow_awaiting_user` (or 409 + decision wording only) →
  `status=blocked` + warn system row; never Failed.
- `requestAgentGraphRefresh`: fire-and-forget graph seed with
  `_agentGraphLoadSeq` stale guard; do **not** invent SSE seq.
- Bump `_agentGraphLoadSeq` on SSE `agent_graph_updated` and control actions
  (Continue/Stop/pause/…) so late blocked HTTP cannot overwrite newer graph.
- `openHistoryRun`: early graph refresh for Continue/Stop card.
- `applyOrchestrationEvent`: settle timeline when `loopStatus === "blocked"`
  (CA-413; status derivation unchanged for BUG-231 running-child contract).

## Provider classification

**Provider-agnostic** — branches on `LoopState.Status` / shared HTTP error codes
only; no `providerKey` switch. Matrix tests: Codex / Claude / Grok.

## Tests (additive only)

| File | Coverage |
|---|---|
| `run63960_blocked_restart_no_gate_reprompt_test.go` | reconstruct ×3 providers; gen high-water; second reconstruct; child+parent blocked; stopped/done settle clear |
| `store.flow-blocked-terminal.test.ts` | blocked settle Thinking; escalate; running non-regression; done non-regression; apply path |
| `store.flow-awaiting-user-409.test.ts` | 409→blocked ×3 providers; non-409 false-positive; history early graph; late refresh vs Continue |

Old suites: `store.flow-terminal`, `store.post-stop-status`, run-45103 cap park,
run-1675 awaiting-user, run-23820 gen — green, **not edited**.

### Known pre-existing failure (not this diff)

`store.history-replay-order.test.ts` expects seq order `[12,7,8,18,17]` but
`orderHistoryReplayEvents` returns wall-clock order `[7,8,12,18,17]`. Verified
red on clean HEAD without this branch's `store.ts` changes (stash). Track
separately; do not edit the old test under safe-fix-contract R1.

## Prior CA claims preserved

- CA-413: blocked timeline settle (Thinking clear)
- CA-412: agent card order durable cohorts
- CA-403: cap park does not cancel submitting hub turn
- CA-23820 / run-23820: reprompt gen high-water
- BUG-231: blocked is actionable pause; running child can still derive running

## Residual risk

- Live re-test required: restart runner+desktop, open run-63960 → expect
  blocked + Continue/Stop (not Failed). Freeform chat should warn, not fail.
- Continue/Stop after restart still relies on existing `continueFlow` /
  `stop` APIs (covered by engine park tests + desktop Continue race test).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-231
change_type: bugfix
summary: After restart, blocked cap park keeps LoopState, skips gate re-run, maps flow_awaiting_user to blocked, seeds graph so Continue/Stop card stays available
# --->8---
