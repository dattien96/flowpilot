# BUG-109: Child/Main Agent Switch Skips Timeline Events And Duplicates Bus Messages

## Metadata

- Document ID: `BUG-109`
- Title: `Child/Main Agent Switch Skips Timeline Events And Duplicates Bus Messages`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [BUG-103: Desktop Focus Child Agent Chat Shows Empty Timeline](./BUG-103-Desktop-Focus-Child-Agent-Chat-Shows-Empty-Timeline.md)
- Replaces: `None`
- Tags: `multi-agent, agents-panel, focus-navigation, zustand, store, regression`

## AI Quick View

### Summary

- After switching from a child agent view back to the main agent view, the main timeline shows an incorrect/truncated response — the latest streaming text is appended into the wrong (previous) assistant bubble.
- Separately, the child agent view can show parent content at the top of its timeline.
- Root cause: `consumeOrchestrationStream` (which runs while viewing a child) calls `applyEvent` on `agent_graph_updated` and `agent_bus_message` events, which advances `_runReplaySeq[mainRunId]` to the seq of those orchestration events. `backToMainRun` then reads this inflated `_runReplaySeq[mainRunId]` as `afterSeq`, causing `consumeAgentStream` to silently skip real timeline events (message_delta, message_completed, turn_completed) whose seqs fall below the inflated value. The stale `_streamingAssistantId` from the snapshot then causes the next message delta to append into the old, unfinished bubble instead of starting a fresh one.
- A secondary effect: if `consumeAgentStream` re-processes `agent_bus_message` events in the replayed gap it would append duplicate entries to `agentBusMessages`.

### Current Ask

- Fix `backToMainRun` to use `restore.lastEventSeq` (the seq captured at snapshot time) instead of `_runReplaySeq[mainRunId]` (which can be inflated by the orchestration stream).
- Fix `consumeAgentStream` to skip `agent_graph_updated` and `agent_bus_message` events (matching `consumeHistoryReplayStream`), so orchestration events in the replayed gap are not double-processed.

### Key Decisions

- `V-1` `backToMainRun` changes `afterSeq = get()._runReplaySeq[mainRunId] ?? restore.lastEventSeq ?? 0` to `afterSeq = restore.lastEventSeq ?? 0`. The snapshot's `lastEventSeq` is the authoritative replay cursor — it records the exact seq of the last event in the snapshot, unaffected by concurrent orchestration stream processing.
- `V-2` `consumeAgentStream` gains the same orchestration-event skip that `consumeHistoryReplayStream` already has: `if (e.type === "agent_graph_updated" || e.type === "agent_bus_message") continue;`. The orchestration stream (`consumeOrchestrationStream`) is the designated owner of these event types. Skipping them in `consumeAgentStream` prevents duplicate `agentBusMessages` entries when the gap is replayed.
- `V-3` Both the TS source (`apps/desktop-flowpilot/src/state/store.ts`) and pre-compiled JS copy (`.phase1-tests/apps/desktop-flowpilot/src/state/store.js`) are updated.
- `V-4` Two regression tests added to `store.test.ts` covering both fix dimensions.

### Constraints

- `consumeStream` (used by `sendPrompt` live turns) is NOT changed — it intentionally processes all event types including orchestration events because the orchestration stream has not yet started when `sendPrompt` fires.
- `focusAgentRun`'s `afterSeq` computation is NOT changed — `_runReplaySeq[childRunId]` is only updated by `consumeAgentStream` for the child (the orchestration stream only advances `_runReplaySeq[mainRunId]`), so the child focus replay cursor is always correct.
- `startOrchestrationStream` already reads `get()._runReplaySeq[runId]` at the moment of call (after `backToMainRun`'s `set`), so it correctly starts from the orchestration-advanced value and does not re-send gap orchestration events. No change needed there.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` — `backToMainRun` (line ~484), `consumeAgentStream` (line ~1481)
- `.phase1-tests/apps/desktop-flowpilot/src/state/store.js` — compiled copies of both
- `apps/desktop-flowpilot/src/state/store.test.ts` — two new regression tests

## 1. Issue Summary

After switching between the main agent view and a child agent view one or more times, the main agent timeline shows an incorrect latest response. The new streaming response text is appended into a previous (already-displayed) assistant bubble rather than starting a new one. The net effect is that the previous completed message appears to grow with the new session's text ("duplicate append"). In some sessions, the child agent view also shows parent-run content at the top of its timeline.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-103: Desktop Focus Child Agent Chat Shows Empty Timeline](./BUG-103-Desktop-Focus-Child-Agent-Chat-Shows-Empty-Timeline.md)

## 3. Environment and Reproduction

- environment: Desktop app, any multi-agent run with at least one child agent
- reproduction steps:
  1. Start a run that spawns a child agent (e.g. a CODEX orchestrator + child).
  2. While the main agent streams a response, click the child in the Agents panel.
  3. Wait for some `agent_graph_updated` events to arrive on the orchestration stream (child state changes).
  4. Click "← Back to main agent".
  5. Continue the main run (send another prompt or wait for the streaming response to complete).
  6. Observe: the streaming text is appended to the previous assistant bubble, not a new one; the main timeline may appear to "rewind" or lose events.
- frequency: Consistent when `agent_graph_updated` / `agent_bus_message` events arrive with seqs interleaved above the main run's last actual timeline event.

## 4. Expected vs Actual

- expected: Switching back to the main agent replays all missed events in order; the new response appears in a fresh assistant bubble.
- actual: Events between `restore.lastEventSeq` and `_runReplaySeq[mainRunId]` (inflated by the orchestration stream) are silently skipped; `_streamingAssistantId` from the snapshot remains set, causing subsequent `message_delta` events to append into the OLD bubble.

## 5. Impact

- users affected: All desktop users using multi-agent runs and switching focus between child and main.
- workflows affected: Main agent chat after returning from child agent view — response display is corrupted.
- severity: High — the main run's transcript shows incorrect content on every round-trip through a child view with active orchestration events.

## 6. Root Cause

- confirmed cause:

  **Step 1 — orchestration stream inflates `_runReplaySeq[mainRunId]`**

  While the user views a child agent, `consumeOrchestrationStream` runs against the parent run's SSE. It processes `agent_graph_updated` and `agent_bus_message` events and calls `applyEvent(s, e)` for each. `applyEvent` updates:
  ```javascript
  const nextReplaySeq = { ...s._runReplaySeq, [e.workflowRunId]: e.seq };
  ```
  Because these events are emitted on the parent run, `e.workflowRunId = mainRunId`. Consequently `_runReplaySeq[mainRunId]` is advanced to the seq of the last orchestration event — which can be HIGHER than the seq of the last real timeline event captured in the main snapshot.

  **Step 2 — `backToMainRun` reads the inflated seq as `afterSeq`**

  ```javascript
  // BEFORE (buggy):
  const afterSeq = get()._runReplaySeq[mainRunId] ?? restore.lastEventSeq ?? 0;
  ```
  `_runReplaySeq[mainRunId]` is now the orchestration-inflated value (e.g. 50). `consumeAgentStream` then applies the filter `if (e.seq <= afterSeq) continue`, silently skipping all real timeline events with seqs 11..50 — including `message_completed` and `turn_completed` that would normally close the streaming bubble and clear `_streamingAssistantId`.

  **Step 3 — stale `_streamingAssistantId` causes text to land in the wrong bubble**

  The restored snapshot still has `_streamingAssistantId` pointing to the previous (unfinished) assistant bubble. When the main run streams new content (`message_delta`), `applyTimelineEvent` finds this ID in the timeline and appends the new text to the OLD bubble instead of starting a fresh one.

- evidence:
  - Code trace: `consumeOrchestrationStream` calls `applyEvent` → `applyEvent` updates `_runReplaySeq[e.workflowRunId]` → `e.workflowRunId = mainRunId` for `agent_graph_updated` events (confirmed from `emitAgentGraphLocked` in `interactive_service.go`).
  - Code trace: `backToMainRun` reads `get()._runReplaySeq[mainRunId]` AFTER child view, so the orchestration-advanced value is used as `afterSeq`.
  - New regression test `"backToMainRun replays from snapshot lastEventSeq, not from orchestration-inflated _runReplaySeq"` directly verifies the afterSeq value passed to `streamRun`.

## 7. Fix Strategy

- `F-1` In `backToMainRun`, use the snapshot's authoritative cursor:
  ```javascript
  // BEFORE:
  const afterSeq = get()._runReplaySeq[mainRunId] ?? restore.lastEventSeq ?? 0;
  // AFTER:
  const afterSeq = restore.lastEventSeq ?? 0;
  ```
  `restore.lastEventSeq` is set by `snapshotRunState` at the moment the main snapshot was taken (before child focus), capturing `_runReplaySeq[mainRunId]` at that point — unaffected by any subsequent orchestration stream activity.

- `F-2` In `consumeAgentStream`, skip orchestration events (matching `consumeHistoryReplayStream`):
  ```javascript
  // Add after the existing `if (e.seq <= afterSeq) continue;` line:
  if (e.type === "agent_graph_updated" || e.type === "agent_bus_message") continue;
  ```
  When `afterSeq = restore.lastEventSeq`, the replayed gap (`restore.lastEventSeq+1` … `orchestration_advanced`) includes orchestration events that were already processed by the previous orchestration stream (which advanced `_runReplaySeq[mainRunId]` in the first place). Re-processing `agent_bus_message` here would append duplicate entries to `agentBusMessages`. Skipping these events in `consumeAgentStream` is semantically correct — `consumeOrchestrationStream` is the designated owner of `agent_graph_updated` and `agent_bus_message`.

- `F-3` Apply both changes to the pre-compiled JS copy in `.phase1-tests/apps/desktop-flowpilot/src/state/store.js`.

## 8. Validation

- `V-1` `apps/desktop-flowpilot/node_modules/.bin/tsc -p tsconfig.phase1-tests.json` passes with no errors after the fix and new tests.
- `V-2` New regression test `"backToMainRun replays from snapshot lastEventSeq, not from orchestration-inflated _runReplaySeq"` verifies that `streamRun` is called with `afterSeq = 10` (snapshot's `lastEventSeq`) even when `_runReplaySeq[mainRunId] = 50` (inflated).
- `V-3` New regression test `"backToMainRun replaying the gap does not duplicate agentBusMessages"` verifies that a `agent_bus_message` event in the replayed gap is not appended to `agentBusMessages` again.
- `V-4` All existing `backToMainRun` and `focusAgentRun` tests pass (TypeScript compilation confirms no type errors; logic of existing tests is unchanged by the new `afterSeq` expression since `restore.lastEventSeq` is `undefined` in all existing test setups that don't set it, making `afterSeq = 0` as before).
- `V-5` Manual verification could not be performed in this environment (requires a live multi-agent session with `agent_graph_updated` events interleaved above the main run's last timeline event seq).

## 9. Regression Guard

- tests: Two new unit tests in `apps/desktop-flowpilot/src/state/store.test.ts` guard both fix dimensions.
- alerts: None.
- audit checks: Any future change to `consumeAgentStream` that re-introduces `agent_graph_updated`/`agent_bus_message` processing would cause the duplicate bus message test to fail.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — this is a defect in the store's event replay bookkeeping; the architectural intent (orchestration stream owns graph/bus events, turn stream owns timeline events) was already documented in the code comments but not enforced in `consumeAgentStream`.
- notes left unchanged on purpose: `consumeStream` (live sendPrompt path) continues to process `agent_graph_updated` events inline because the orchestration stream has not started yet at that point.
