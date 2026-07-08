# CA-256: Delete Active Chat Reuses Full `resetRun()` Instead Of A Partial Hand-Rolled Reset

## What changed

- `apps/desktop-flowpilot/src/state/store.ts`: `deleteHistoryRun`'s `wasActive` branch now calls `get().resetRun()` instead of a hand-rolled `set({...})` that only cleared chat-turn fields (`timeline`, `artifacts`, `pendingApprovals`/`pendingQuestions`, etc.). `resetRun()` additionally clears `mainRunId`, `activeAgentRunId`, `agentRuns`, `agentGraphSnapshot`, `agentBusMessages`, `workflowStepRuntime`, `workflowStepRuntimeMeta`, `_runSnapshots`, `_runReplaySeq`, and cancels the live orchestration/agent-focus/history-replay streams.
- `apps/desktop-flowpilot/src/state/store.test.ts`: added a regression test seeding Flow Timeline and Agents panel state on the active run, then asserting `deleteHistoryRun` clears all of it (not just the chat timeline).

## Why

User-reported UI bug: deleting the currently-open chat cleared the main chat/timeline area but left the Flow Timeline sidebar and Agents panel showing the deleted chat's steps/agents, because both panels resolve their run via `mainRunId ?? runId` and the old reset never cleared `mainRunId`. See [BUG-258](../requirements/09-BugFix/done/BUG-258-Delete-Active-Chat-Leaves-Stale-Flow-Timeline-And-Agents-Panel.md).

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-258
change_type: bugfix
summary: deleting the active chat now reuses resetRun() so Flow Timeline and Agents panel state clears along with the chat timeline
# --->8---
