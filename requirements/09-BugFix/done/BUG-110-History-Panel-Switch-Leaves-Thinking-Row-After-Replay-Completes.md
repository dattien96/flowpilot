# BUG-110: History Panel Switch Leaves Thinking Row After Replay Completes

## Metadata

- Document ID: `BUG-110`
- Title: `History Panel Switch Leaves Thinking Row After Replay Completes`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [BUG-109: Child/Main Agent Switch Skips Timeline Events](./BUG-109-Child-Main-Agent-Switch-Skips-Timeline-Events-And-Duplicates-Bus-Messages.md)
- Replaces: `None`
- Tags: `multi-agent, history-panel, orchestration-stream, thinking-row, zustand, store`

## AI Quick View

### Summary

- After switching between chats in the history panel (clicking a different run and then returning to the original), a "Thinking..." row persists in the timeline even though the run has fully completed.
- Root cause: `consumeOrchestrationStream` calls `applyEvent(s, e)` which calls `applyTimelineEvent`. For `agent_graph_updated` and `agent_bus_message` events, `applyTimelineEvent` unconditionally adds a `thinking` row (because `shouldKeepThinking = true` for these event types). After `consumeHistoryReplayStream` finishes and settles the timeline to a completed state (no thinking row), the orchestration stream then processes its own historical events and re-adds the thinking row.
- A secondary symptom is a perceived "duplicate response" when switching back to a running run — this is likely the same timing issue manifesting differently.

### Current Ask

- Make `consumeOrchestrationStream` update ONLY agent graph state (`agentRuns`, `agentGraphSnapshot`, `agentBusMessages`, `_runReplaySeq`) without touching the timeline. The timeline is owned by `consumeHistoryReplayStream` / `consumeAgentStream` / `consumeStream`.

### Key Decisions

- `V-1` New `applyOrchestrationEvent` function handles graph/bus events without calling `applyTimelineEvent`. This is the authoritative separation: the orchestration stream is a pure graph-data subscriber; timeline visuals are always managed by whichever turn-replay stream is active.
- `V-2` `applyEvent` is unchanged — it correctly calls `applyTimelineEvent` for all paths that DO own the timeline (`consumeStream`, `consumeHistoryReplayStream`, `consumeAgentStream`).
- `V-3` Two regression tests confirm: (a) the thinking row does not appear after replay completes and the orchestration stream emits `agent_graph_updated`; (b) timeline content is unchanged by orchestration events while `agentRuns`/`agentBusMessages` are correctly updated.

### Constraints

- `consumeStream` (live `sendTurn` path) is NOT changed. During a live turn, `agent_graph_updated` events flow through `consumeStream` which correctly manages the thinking row (run is active, thinking is appropriate). Orchestration stream is not started until after the turn completes.
- `applyEvent` is NOT changed so all other callers remain unaffected.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` — `consumeOrchestrationStream` (line ~1514), new `applyOrchestrationEvent` function
- `apps/desktop-flowpilot/src/state/store.test.ts` — two new regression tests

## 1. Issue Summary

After switching from the current chat to a different chat in the history panel and then switching back, the timeline shows "Thinking..." even though the run has fully completed. Alternatively, on some timing paths the user sees what appears to be a duplicate response entry.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-109: Child/Main Agent Switch Skips Timeline Events](./BUG-109-Child-Main-Agent-Switch-Skips-Timeline-Events-And-Duplicates-Bus-Messages.md)

## 3. Environment and Reproduction

- environment: Desktop app, any run with at least one child agent (so `agent_graph_updated` events exist in the SSE stream)
- reproduction steps:
  1. Start or view a run that has spawned at least one child agent.
  2. Click on a different chat in the history panel.
  3. Click back to the original run.
  4. Observe: "Thinking..." appears at the bottom of the timeline even though the run is complete. OR, alternatively, a duplicate response entry appears.
- frequency: Consistent when the run has `agent_graph_updated` events in its SSE stream and the orchestration stream processes them after the history replay has finished.

## 4. Expected vs Actual

- expected: After switching back to a completed run via the history panel, the timeline is stable with no thinking row.
- actual: The orchestration stream processes its `agent_graph_updated` events after the history replay settles, re-adding the thinking row via `applyTimelineEvent`.

## 5. Impact

- users affected: All desktop users with multi-agent runs who switch between chats in the history panel.
- workflows affected: History panel navigation with any run that has child agents.
- severity: Medium — visual artifact that persists until the next state update clears it (e.g., another navigation or interaction).

## 6. Root Cause

- confirmed cause:

  **Step 1 — `openHistoryRun` starts two concurrent SSE subscribers**

  ```typescript
  void consumeHistoryReplayStream(handle.runId, handle.status, client.streamRun(handle.runId, 0, ...))
  startOrchestrationStream(handle.runId, client, set, get);
  ```

  Both start from `afterSeq = 0` (since `_runReplaySeq` is reset to `{}` on each `openHistoryRun`). They are independent HTTP connections to the same SSE endpoint.

  **Step 2 — `consumeHistoryReplayStream` finishes first for completed runs**

  For `resumedStatus === "completed"`, `consumeHistoryReplayStream` breaks at `turn_completed`, calls `settleHistoryReplayPendingState`, and the thinking row is gone.

  **Step 3 — `consumeOrchestrationStream` then processes `agent_graph_updated` events**

  `consumeOrchestrationStream` calls `set((s) => applyEvent(s, e))`. `applyEvent` calls `applyTimelineEvent`. In `applyTimelineEvent`, `shouldKeepThinking = true` for `agent_graph_updated` (it is not `turn_completed`/`turn_failed`/`permission_required`/`user_question_required`). So `applyTimelineEvent` adds a thinking row to the already-settled timeline.

  **Step 4 — thinking row persists**

  No subsequent call to `settleTerminalReplayVisuals` strips the thinking row (the orchestration stream never calls it). The thinking row remains visible until the next state update.

- evidence:
  - Code trace: `consumeOrchestrationStream` line ~1514 calls `applyEvent` → `applyTimelineEvent` → thinking row added.
  - `applyTimelineEvent` switch has no case for `agent_graph_updated` → falls through to `return finalize(timeline)` where `finalize` adds the thinking row when `shouldKeepThinking = true`.
  - New regression test `"openHistoryRun orchestration stream does not add thinking row after history replay completes"` directly triggers the race and asserts no thinking row.

## 7. Fix Strategy

- `F-1` Add `applyOrchestrationEvent` function that handles `agent_graph_updated` and `agent_bus_message` WITHOUT calling `applyTimelineEvent`:

  ```typescript
  function applyOrchestrationEvent(s: AppState, e: ProviderEventDTO): Partial<AppState> {
    const nextReplaySeq = { ...s._runReplaySeq, [e.workflowRunId]: e.seq };
    if (e.type === "agent_graph_updated") {
      return {
        agentRuns: e.agentGraphSnapshot.runs,
        agentGraphSnapshot: e.agentGraphSnapshot,
        agentBusMessages: e.agentGraphSnapshot.busMessages,
        _runReplaySeq: nextReplaySeq,
      };
    }
    if (e.type === "agent_bus_message") {
      return {
        agentBusMessages: [...s.agentBusMessages, e.agentBusMessage],
        agentGraphSnapshot: s.agentGraphSnapshot
          ? { ...s.agentGraphSnapshot, busMessages: [...s.agentGraphSnapshot.busMessages, e.agentBusMessage] }
          : s.agentGraphSnapshot,
        _runReplaySeq: nextReplaySeq,
      };
    }
    return {};
  }
  ```

- `F-2` In `consumeOrchestrationStream`, replace `set((s) => applyEvent(s, e))` with `set((s) => applyOrchestrationEvent(s, e))`.

- `F-3` Apply both changes to the pre-compiled JS copy in `.phase1-tests/apps/desktop-flowpilot/src/state/store.js` (via TypeScript compilation).

## 8. Validation

- `V-1` `tsc -p tsconfig.phase1-tests.json --noEmit` passes with no errors after the fix and new tests.
- `V-2` New regression test `"openHistoryRun orchestration stream does not add thinking row after history replay completes"` verifies the thinking row is absent both after history replay and after the orchestration stream processes `agent_graph_updated`.
- `V-3` New regression test `"openHistoryRun orchestration stream updates agent graph without touching timeline content"` verifies that `agentRuns`/`agentBusMessages` are correctly updated while timeline content is unchanged.
- `V-4` Manual verification could not be performed in this environment (requires a live multi-agent session with history panel navigation).

## 9. Regression Guard

- tests: Two new unit tests in `apps/desktop-flowpilot/src/state/store.test.ts` guard both fix dimensions.
- alerts: None.
- audit checks: Any future revert of `consumeOrchestrationStream` to use `applyEvent` would cause the thinking-row test to fail.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — this is a defect in the event-to-state mapping for the orchestration stream; the design intent (orchestration stream owns graph data, turn stream owns timeline) was correct but not enforced in the implementation.
- notes left unchanged on purpose: `consumeStream` (live `sendTurn` path) continues to use `applyEvent` for `agent_graph_updated` — during live turns, the thinking row behavior is correct (no separate orchestration stream is running at that time).
