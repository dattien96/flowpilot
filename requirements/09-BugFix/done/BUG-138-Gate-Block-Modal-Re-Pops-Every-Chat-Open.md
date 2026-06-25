# BUG-138: Gate Block Modal Re-Pops On Every Chat Open

## Metadata

- Document ID: `BUG-138`
- Title: `Gate Block Modal Re-Pops On Every Chat Open`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: none
- Related Documents: [BUG-137](./BUG-137-Navigator-Spinner-Persists-For-Gate-Blocked-Inactive-Chat.md), [CA-126](../../../change-audit/CA-126-fix-gate-block-ui-and-r-bug-action.md)
- Replaces: none
- Tags: `context-regression-engine, ui, flow-gate, modal, regression`

## AI Quick View

### Summary

- After a gate hard-block, the "Flow gate blocked this step" modal correctly appears once (live turn). But every subsequent time the user opens that chat from history, the modal re-appears — it should only appear on the live turn.
- Root cause: a race between `consumeHistoryReplayStream` (which runs with `_historyReplaying=true`) and `consumeOrchestrationStream` (which starts from seq 0 at the same time). The orchestration stream uses a static `afterSeq=0` watermark. By the time the history replay finishes and sets `_historyReplaying=false`, the orchestration stream re-delivers the `flow_gate_violation` event and `applyEvent` sets `gateBlock` — popping the modal.
- Affects E2E-8 (r-tests/r-reg) and E2E-11 (any hard block scenario).

### Current Ask

- Prevent the orchestration stream from re-processing events that were already handled by the concurrent history replay.

### Key Decisions

- `D-1` In `consumeOrchestrationStream`, non-agent events already pass a static `afterSeq` watermark. Add a second, **dynamic** watermark: skip any event whose seq is ≤ `get()._runReplaySeq[runId]` (updated continuously by the replay stream as it processes events). This ensures events processed by the replay are invisible to the orchestration stream once `_historyReplaying` turns false.
- `D-2` Agent events (`agent_graph_updated`, `agent_bus_message`) are unaffected — they route to `applyOrchestrationEvent` which does not set `gateBlock`.

### Constraints

- Must not break the CP-35 reprompt path: reprompt events (`turn_started`, `message_delta`, etc.) must still reach the orchestration stream. They arrive with seqs HIGHER than the `flow_gate_violation` seq, so they are not skipped by the dynamic watermark. ✓
- Must not skip genuinely live events after the replay completes. Live events have seqs above the last replay event, so `e.seq > _runReplaySeq` → not skipped. ✓

### Open Questions

- none

### Source Refs

- Code: `apps/desktop-flowpilot/src/state/store.ts` (`consumeOrchestrationStream`, `applyEvent`).

## 1. Issue Summary

Opening a gate-blocked chat from history pops the "⛔ Flow gate blocked this step" modal every time. The modal is intended to show only once — when the block fires live during a turn.

## 2. Parent Links

- impacted coding plan: CP-35
- impacted tech design: SD-20 §3 (UX: block → modal)
- impacted system spec: SS-14

## 3. Environment and Reproduction

- environment: Desktop app, any chat that received a hard gate block
- reproduction steps: (1) Trigger a gate block (e.g. E2E-8: failing test). (2) Dismiss the modal. (3) Switch to a different chat. (4) Switch back to the blocked chat. Observe: modal re-appears.
- frequency: 100%

## 4. Expected vs Actual

- expected: Modal appears exactly once, on the live block. Reopening the chat shows the inline warn card only (no modal).
- actual: Modal re-appears on every open of the blocked chat.

## 5. Impact

- users affected: Any user who hits a hard gate block and reopens the chat
- workflows affected: Context & Regression Engine (CP-35)
- severity: High — repeated interruption; undermines trust in the gate UX

## 6. Root Cause

- hypothesis: Orchestration stream processes `flow_gate_violation` after `_historyReplaying` turns false
- confirmed cause: `openHistoryRun` starts `consumeHistoryReplayStream` and `consumeOrchestrationStream` concurrently, both from seq 0. `_historyReplaying=true` during the replay. When the replay finishes, its `finally` sets `_historyReplaying=false`. The orchestration stream is still consuming the same event log; when it hits `flow_gate_violation` (after `_historyReplaying` is now false), `applyEvent` sets `gateBlock`.
- evidence: Code trace: `openHistoryRun` → `consumeHistoryReplayStream` + `startOrchestrationStream` in parallel. Static `afterSeq=0` means the orchestration stream replays all events. `_historyReplaying` is false by the time the orchestration stream's async event loop processes `flow_gate_violation`.

## 7. Fix Strategy

- `F-1` In `consumeOrchestrationStream`, inside the non-agent event branch, add a dynamic watermark check before calling `applyEvent`:
  ```typescript
  if (e.seq <= (get()._runReplaySeq[runId] ?? afterSeq)) continue;
  set((s) => applyEvent(s, e));
  ```
  `_runReplaySeq[runId]` is continuously updated by the replay stream as it processes events. Once the replay has processed `flow_gate_violation` (updating `_runReplaySeq` to its seq), the orchestration stream's dynamic check will see `e.seq <= _runReplaySeq` and skip it — regardless of whether `_historyReplaying` is still true or already false.

## 8. Validation

- `V-1` Open a gate-blocked chat → no modal. Inline warn card still shows in timeline. ✓ (manual)
- `V-2` Live gate block still shows modal on the turn it fires. ✓ (manual — modal fires from `consumeStream`, not orchestration)
- `V-3` Gate reprompt events (turn_started, message_delta) still appear in timeline after opening a reprompted chat. ✓ (reprompt events have seqs above `_runReplaySeq`).

## 9. Regression Guard

- tests: `timelineReducer.test.ts` covers the `flow_gate_violation` status settlement. Store-level test for orchestration stream dedup not added (would require mocking the stream).
- alerts: none
- audit checks: Ensure `_runReplaySeq[runId]` is updated by `applyEvent` on every event (it is, via `nextReplaySeq`).

## 10. Follow-Up Document Updates

- SD-20 §1 (where the gate runs) and §3 (UX behavior): no content change needed; the fix is a frontend implementation detail of the replay race condition.
