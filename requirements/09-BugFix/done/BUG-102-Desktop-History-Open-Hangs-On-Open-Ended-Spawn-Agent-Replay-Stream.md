## Metadata

- Document ID: `BUG-102`
- Title: `Desktop History Open Hangs On Open Ended Spawn Agent Replay Stream`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-20`
- Last Updated: `2026-06-20`
- Parent Documents: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: none
- Related Documents: [BUG-074: Desktop History Replay Leaves Resolved Approvals in Pending State](./BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md), [BUG-046: Desktop History Replay Loses User Prompts](./BUG-046-Desktop-History-Replay-Loses-User-Prompts.md), [BUG-101: Restarted Desktop Session Did Not Show The Spawn Agent Child Run](./BUG-101-Restarted-Desktop-Session-Did-Not-Show-The-Spawn-Agent-Child-Run.md)
- Replaces: none
- Tags: desktop, history, replay, stream, regression, severity-high

## AI Quick View

### Summary

- Clicking an old spawn-agent chat from desktop history could freeze the app because the history open path waited on a replay stream that never closes.
- The stream is an SSE source, so it can stay open indefinitely even after the visible replay is complete.
- Follow-up failure: making replay non-blocking was not enough because the old replay/orchestration SSE sockets stayed open while idle; after switching once and sending a later prompt, the new send stream could hang.
- The final fix makes replay non-blocking and actively aborts superseded history/orchestration streams before opening a new history run or sending a new prompt.

### Current Ask

- Capture the history-open hang as a standalone bugfix record.

### Key Decisions

- `V-1` History open must return to the UI immediately after attaching the replay stream.
- `V-2` Replay cleanup must remain separate from the live orchestration stream.
- `V-3` Sequence counters are not sufficient for open-ended SSE streams; superseded streams must receive an abort signal so their HTTP connections close while idle.

### Constraints

- Do not change the provider SSE contract.
- Do not make history open wait for the replay stream to close.
- Do not leave background history/orchestration streams open after the user switches chats or starts a new turn.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
- `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
- `apps/desktop-flowpilot/src/types/contract.ts`

## 1. Issue Summary

The user clicked an old spawn-agent chat from desktop history and the whole app appeared to hang. The first fix made the selection return once, but after waiting and sending a new prompt the app could hang again because the old background streams were still open.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, resumed Codex chat, existing history entry, no server restart required
- reproduction steps:
  1. Finish a spawn-agent chat.
  2. Click an older history chat that still has a spawn-agent run.
  3. Switch once successfully.
  4. Wait briefly, then send a new prompt.
  5. Observe the send hang.
- frequency: deterministic when superseded replay/orchestration SSE streams remain open

## 4. Expected vs Actual

- expected: switching history chats returns immediately, old replay/orchestration streams close, and sending a new prompt starts a clean turn stream
- actual: the first switch could return, but old replay/orchestration streams stayed open and later prompt sends could hang

## 5. Impact

- users affected: desktop users opening old chats
- workflows affected: chat switching, run review, post-restart recovery, follow-up prompts on resumed chats
- severity: high, because the entire app looks frozen

## 6. Root Cause

- hypothesis: the app was waiting on or leaking streams that are designed to stay open
- confirmed cause:
  - first cause: `openHistoryRun` awaited `consumeStream(client.streamRun(runId, 0))`, but `streamRun` is an SSE feed and can remain open forever
  - second cause: after replay was moved to the background, stale history replay and orchestration streams were only protected by sequence counters. Those counters stop stale events from applying, but they do not close an idle `fetch` stream until another event arrives.
- evidence: `HttpWsRunnerClient.openStream` already aborts fetch when the consumer stops iterating, but the store did not hold a cancellation handle for history replay or orchestration streams. The final fix adds `AbortSignal` support to `streamRun` and aborts superseded streams on history open and before `sendPrompt`.

## 7. Fix Strategy

- `F-1` Move history replay onto a background task.
- `F-2` Keep live orchestration streaming independent of replay.
- `F-3` Preserve the stale-pending cleanup behavior after replay settles.
- `F-4` Add optional `AbortSignal` support to `RunnerClient.streamRun`, `HttpWsRunnerClient.streamRun`, and `MockRunnerClient.streamRun`.
- `F-5` Store active history replay and orchestration controllers in the desktop store and abort them before opening another history run or sending a new prompt.

## 8. Validation

- `V-1` `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `V-2` `npx tsc -p ../../tsconfig.phase1-tests.json` passes in `apps/desktop-flowpilot`.
- `V-3` `openHistoryRun returns after attaching an open-ended history stream` is added as a regression test.
- `V-4` `sendPrompt aborts an open-ended history replay stream before sending` is added as a regression test.

## 9. Regression Guard

- tests: desktop store regressions for open-ended replay and prompt send after open-ended replay
- alerts: none
- audit checks: history open should never block on SSE completion; superseded history/orchestration streams must be explicitly aborted

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: the runner SSE contract stays open-ended; only the desktop stream ownership policy changed
