# BUG-258: Delete Active Chat Leaves Stale Flow Timeline And Agents Panel

## Metadata

- Document ID: `BUG-258`
- Title: `Delete Active Chat Leaves Stale Flow Timeline And Agents Panel`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [Task-077: Delete Chat History](../../08-Task/done/Task-077-Delete-Chat-History.md), [Task-086: Delete Chat Cascades To Child Agents](../../08-Task/done/Task-086-Delete-Chat-Cascades-To-Child-Agents.md), [CP-17: Workflow Chat And Session](../../07-Coding-Plan/done/CP-17-Workflow-Chat-And_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-248: Stop Leaves Main Run Permanently Stuck Running](./BUG-248-Stop-Leaves-Main-Run-Permanently-Stuck-Running.md), [CA-256: Delete Active Chat Reuses Full resetRun()](../../../change-audit/CA-256-delete-active-chat-reuses-full-reset.md)
- Replaces: `none`
- Tags: `desktop, delete-chat, flow-mode, agents-panel, state-reset, regression`

## AI Quick View

### Summary

- User report: deleting the chat that is currently open is expected to return the desktop UI to an empty new-chat state. Instead, the chat/timeline area clears, but the Flow Timeline sidebar (round/step cards) and the Agents panel (sub-agent list, main orchestrator row) keep showing the deleted chat's content.
- Root cause: `deleteHistoryRun`'s `wasActive` branch (`store.ts`) hand-rolled its own partial `set({...})` reset instead of calling the store's existing `resetRun()` action. The hand-rolled reset cleared chat-turn fields (`timeline`, `artifacts`, `pendingApprovals/Questions`, etc.) but omitted every field the Flow Timeline and Agents panel actually read — `mainRunId`, `activeAgentRunId`, `agentRuns`, `agentGraphSnapshot`, `agentBusMessages`, `workflowStepRuntime`, `workflowStepRuntimeMeta`, `_runSnapshots`, `_runReplaySeq` — and never cancelled the live orchestration/agent-focus/history-replay streams.
- Both panels fall back to `mainRunId ?? runId` (`FlowTimelineSidebar.tsx`, `AgentsPanel.tsx`). Since the hand-rolled reset cleared `runId` but not `mainRunId`, both panels still resolved a run to render against and kept displaying the deleted chat's Flow steps and agent list.
- Fix: replace the hand-rolled `set({...})` in the `wasActive` branch with a call to `get().resetRun()`, the same full reset already used by `selectProject` and `setChatMode` when starting a genuinely new context — eliminating the second, drifted copy of the reset logic instead of patching its field list by hand.

### Current Ask

- Fixed and verified: deleting the active chat now fully returns the desktop UI to an empty new-chat state (chat, Flow Timeline, and Agents panel all clear together).

### Key Decisions

- `D-1` Reuse `resetRun()` rather than adding the missing fields to the hand-rolled `set({...})` call. `resetRun()` is the single existing "return to empty new chat" primitive (already used by `selectProject` on project switch and `setChatMode` on mode switch); keeping two independently-maintained copies of the same reset is what caused this bug, and would keep causing it if a third field is later added to one but not the other.
- `D-2` No change to the non-active-run path of `deleteHistoryRun` (the `runHistory` filter and the API call / error-recovery re-fetch) — those are correct and unrelated to this bug.

### Constraints

- Scoped to the `wasActive` branch of `deleteHistoryRun`; the optimistic `runHistory` removal and the `client.deleteRun` call/error-recovery path are unchanged.
- `resetRun()` additionally resets `chatStartMode`, `chatSourceDocId`, `flowRef`, `builtinOrchestrationOptions`, and `selectedModel` (via `pickDefaultModel`) — a superset of what the old hand-rolled reset touched. This is intentional: deleting the active chat should behave exactly like starting a brand new chat, which is what `resetRun()` is already defined to mean elsewhere in the store.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` `deleteHistoryRun` (line ~1384), `resetRun` (line ~1603).
- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx` (line 13-21): reads `mainRunId ?? runId`, `workflowStepRuntime`, `workflowStepRuntimeMeta`, `agentGraphSnapshot.loopState`.
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` (line 44-98): reads `mainRunId ?? runId`, `agentRuns`, `_runSnapshots`, `workflowStepRuntimeMeta`.
- New test: `store.test.ts` `"deleteHistoryRun on the active chat clears Flow Timeline and Agents panel state, not just the timeline (BUG-258)"`.

## 1. Issue Summary

Opening a chat, then deleting that same (currently open) chat from history, was expected to return the desktop workspace to a completely empty new-chat state. Instead the main chat/timeline area went empty, but the Flow Timeline sidebar kept showing the deleted chat's round/step cards (e.g. "Round 1", coder/reviewer step statuses) and the Agents panel kept showing the deleted chat's sub-agent list (main orchestrator row, recently-closed agent rows).

## 2. Parent Links

- impacted coding plan: [CP-17: Workflow Chat And Session](../../07-Coding-Plan/done/CP-17-Workflow-Chat-And_Session.md)
- impacted tech design: none dedicated — delete-chat behavior is defined at the Task level (see Task links below)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md) (implied chat-lifecycle contract; delete is one of its lifecycle transitions)

## 3. Environment and Reproduction

- environment: Desktop app, any provider/chat mode, a chat with at least one completed Flow-mode step or spawned sub-agent (Flow Timeline sidebar and Agents panel populated).
- reproduction steps:
  1. Open a chat that has Flow Timeline steps and/or Agents panel entries (e.g. a Review Loop run with coder/reviewer steps).
  2. From the same chat's history row (or an equivalent delete affordance), delete the currently-open chat.
  3. Observe: the main chat/timeline area clears to empty, but the Flow Timeline sidebar still shows the deleted chat's round/step cards, and the Agents panel still shows its sub-agent list.
- frequency: deterministic — reproduces every time the deleted chat is the currently-active one and has any Flow/Agents state populated.

## 4. Expected vs Actual

- expected: deleting the currently-open chat returns the whole workspace — chat, Flow Timeline sidebar, and Agents panel — to the same empty state as starting a brand new chat.
- actual: only the chat/timeline area cleared; Flow Timeline and Agents panel kept rendering the deleted chat's data because their backing state (`mainRunId`, `agentRuns`, `agentGraphSnapshot`, `workflowStepRuntime`, `workflowStepRuntimeMeta`, `_runSnapshots`) was never cleared, and both panels fall back to `mainRunId ?? runId` to resolve which run to render.

## 5. Impact

- users affected: any user deleting a chat that is currently open and has Flow-mode steps or spawned agents — a common case, not an edge case.
- workflows affected: chat history delete (Task-077/Task-086), Flow Timeline sidebar, Agents panel.
- severity: medium — no data loss (the backend delete and history-list removal both work correctly) and no functional deadlock, but confusing/incorrect UI state that could lead a user to believe the deleted chat is still running or mistake its stale content for a new chat's state.

## 6. Root Cause

- hypothesis: the `wasActive` reset branch in `deleteHistoryRun` was written before (or independently of) the Flow-mode/Agents-panel fields existed in `AppState`, and was never updated to cover them as those features were added.
- confirmed cause: direct comparison of `deleteHistoryRun`'s `wasActive` branch against `resetRun()` (`store.ts`) shows the hand-rolled `set({...})` in `deleteHistoryRun` omits `mainRunId`, `activeAgentRunId`, `agentRuns`, `agentGraphSnapshot`, `agentBusMessages`, `workflowStepRuntime`, `workflowStepRuntimeMeta`, `_runSnapshots`, `_runReplaySeq`, and never calls `cancelHistoryReplayStream`/`cancelOrchestrationStream`/`cancelAgentFocusStream` — all of which `resetRun()` does. `FlowTimelineSidebar.tsx` and `AgentsPanel.tsx` both key off `mainRunId ?? runId`; since `mainRunId` survived the hand-rolled reset, both panels kept resolving a run and rendering its (stale) data.
- evidence: added a regression test that seeds `mainRunId`, `agentRuns`, `agentGraphSnapshot`, `workflowStepRuntime`, `workflowStepRuntimeMeta`, and `_runSnapshots` for the active run, calls `deleteHistoryRun` on it, and asserts every one of those fields is cleared — this test failed before the fix (fields retained their seeded values) and passes after.

## 7. Fix Strategy

- `F-1` `store.ts` `deleteHistoryRun`: in the `wasActive` branch, replace the hand-rolled `set({...})` (13 fields, missing Flow/Agents state and stream cancellation) with a call to `get().resetRun()` — the store's existing single source of truth for "return to an empty new chat", already used by `selectProject` and `setChatMode`.

## 8. Validation

- `V-1` `apps/desktop-flowpilot`: `tsc --noEmit` — clean.
- `V-2` New test added to `store.test.ts`: `"deleteHistoryRun on the active chat clears Flow Timeline and Agents panel state, not just the timeline (BUG-258)"` — seeds `mainRunId`, `activeAgentRunId`, `agentRuns`, `agentGraphSnapshot`, `workflowStepRuntime`, `workflowStepRuntimeMeta`, and `_runSnapshots` on the active run, calls `deleteHistoryRun`, and asserts all of them (plus `runId`, `status`, `timeline`, `runHistory`) are cleared to their empty/idle defaults.
- `V-3` Ran the full `store.test.ts` suite via an isolated `tsc` (CommonJS output) + Node `--test` + alias-resolution-hook harness (this repo's `test:phase1` script does not itself execute `store.test.ts`, same as noted in BUG-247/BUG-248): 78 tests, 76 passed. The 2 failures (`"selectProject resets the active chat run when switching projects"`, `"sendPrompt aborts an open-ended history replay stream before sending"`) are the same pre-existing environment-only failures documented in BUG-248 V-5 (`localStorage is not defined` — no DOM in the isolated harness), confirmed unrelated to this change by inspecting the failure output directly (`ReferenceError: localStorage is not defined`, thrown from unrelated code paths, not from `deleteHistoryRun`/`resetRun`). The new BUG-258 test passed.
- `V-4` GitNexus `impact` CLI run before editing (MCP tools were unavailable in this session, so `npx gitnexus impact <target> --repo flowpilot` was used instead): both `deleteHistoryRun` and `resetRun` reported `"risk": "LOW"`, `"impactedCount": 0` upstream — confirmed safe to edit.

## 9. Regression Guard

- tests: `store.test.ts` `"deleteHistoryRun on the active chat clears Flow Timeline and Agents panel state, not just the timeline (BUG-258)"`.
- audit checks: `gitnexus_detect_changes()` MCP tool was unavailable in this session (only the GitNexus CLI was reachable); `gitnexus impact` was used pre-edit instead (see V-4). No post-edit `gitnexus_detect_changes()` scope check was run.

## 10. Follow-Up Document Updates

- None. This is a pure bug fix that makes `deleteHistoryRun` reuse an existing, already-correct reset primitive (`resetRun()`); it does not change the delete-chat feature's contract (Task-077/Task-086) or introduce new behavior beyond what "delete the active chat" was always supposed to do.
