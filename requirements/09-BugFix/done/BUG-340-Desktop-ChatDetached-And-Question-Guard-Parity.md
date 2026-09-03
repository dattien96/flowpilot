# BUG-340 — Desktop detached reattach, 409 defer, and question-pending guard parity with TUI

## Metadata

- Document ID: `BUG-340`
- Title: `Desktop detached reattach, 409 defer, and question-pending guard parity with TUI`
- Phase: `bugfix`
- Status: `done`
- Owner: `codex`
- Reviewers: `codex`
- Created: `2026-08-31`
- Last Updated: `2026-09-02`
- Parent Documents: `CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat`, `SD-26-Chat-Continuity-Ssot`
- Child Documents: ``
- Related Documents: `CA-699-tui-detached-reattach-restore-by-chat-task315-slice3`, `CA-700-desktop-chat-switch-surface-task316`, `BUG-339-TUI-Posture-Active-Not-Persisted-After-Cross-Provider-Tab`
- Replaces: ``
- Tags: `chat-history, chat-ui, desktop, parity, CP-59`

## AI Quick View

### Summary

- Desktop `openHistoryRun` + `sendPrompt` used `resumeRun` + `sendTurn` on a restored chat whose legs are all terminal (no active leg). The runner's `chat_no_active_leg` envelope seed only fires on `startRun(chatId, switchFromRunId)` — the Nam memory loss after restart repro. TUI already reattaches via `startRun`.
- `confirmProviderSwitch` always called `switchChatProvider` even when the chat is detached (F3) or when a `ask_user` question/approval is pending (C2), producing `409` + `question_expired` instead of the TUI's defer notice / busy error.
- The `409 chat_no_active_leg` error from the switch endpoint surfaced as a red error instead of the TUI's "will reattach on next prompt" defer.

### Current Ask

- Make Desktop `sendPrompt` on a detached chat mint a fresh leg via `startRun(chatId, switchFromRunId)` so the first prompt carries the prior turns envelope (same as TUI `cmdReattachChat`). Make `confirmProviderSwitch` defer locally when detached (no endpoint call) and block when a question/approval is pending, and treat `409 chat_no_active_leg` as detached defer. Mirror TUI's `setChatPosture` question guard.

### Key Decisions

- `V-1` Add `chatDetached` boolean to `store.ts` (mirrors TUI `chatDetached`), derived in `openHistoryRun` from `handle.status` terminal when `chatId` exists and `runKind` is chat.
- `V-2` `sendPrompt`: when `chatDetached && chatId && runId`, call `startRun({ chatId, switchFromRunId: runId, ...current provider/model})` before the turn, clear `chatDetached`, and send the prompt on the new leg. Provider-agnostic: `chatId` only.
- `V-3` `confirmProviderSwitch`: check `pendingQuestions`/`pendingApprovals` first → busy error (no endpoint call); check `chatDetached && chatId` → local `selectedProvider`/`selectedModel` apply + defer notice + `loadSkills` (no endpoint); catch `409 chat_no_active_leg` → same defer. `resetRun` and successful switch/legacy paths clear `chatDetached`.
- `V-4` `setChatPosture`: block when a question/approval is pending (same message as TUI) — posture Tab must not steal the pending card.

### Constraints

- Additive tests only; no edits to `store.chatSwitch.test.ts` or other legacy store tests. Provider-agnostic: `chatId` join only, no `providerKey` branch — verified by a table test over `grok/codex/claude`.
- `openHistoryRun` still sets `chatMode` from `historyItem.runKind` (BUG-170) — detached flag is orthogonal.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts:54` `buildPriorChatTimeline` (F4, already Desktop-done), `store.ts:1523` `sendPrompt`, `store.ts:1102` `confirmProviderSwitch`, `store.ts:1472` `setChatPosture`
- `apps/local-runner/internal/tui/app/chat_switch.go:158` `routePostureSwitch`/`routeProviderSwitch` (TUI reference), `CA-699` reattach, `CP-59-Test-Steps` F2/F3/C2

## 1. Issue Summary

Desktop could re-open a 3-leg chat via `/open` and show the full transcript (F4, already done), but the next prompt was sent via `sendTurn` on the old terminal runId. The runner's reattach envelope (which seeds prior turns for the model) only runs on `startRun` with `chatId` + `switchFromRunId`, so the model forgot `Nam`. Provider chip switches while the chat was detached or while a question was pending hit the switch endpoint and `409` instead of the TUI's local defer / busy guard.

## 2. Parent Links

- impacted coding plan: `CP-59` F2 (reattach), F3 (detached Tab defer), C2 (question guard)
- impacted tech design: `SD-26` §10 detached chats, §5.1 chat identity
- impacted system spec: `SS-05` chat history contract

## 3. Environment and Reproduction

- environment: Desktop `store.ts` at `CA-700`, runner always-ON, `MockRunnerClient` with `chatTimeline` mocked
- reproduction steps:
  1. TUI: create a 3-leg chat `cht_a` (opencode → grok → codex), quit TUI, run `just chat-dev` to persist.
  2. Desktop: `openHistoryRun("run-3", {runKind:"chat", chatId:"cht_a"})` where `run-3` status is `completed` → `chatDetached` should be true but was undefined; timeline shows prior legs via `buildPriorChatTimeline`.
  3. Desktop: `sendPrompt("ban biet gi ve toi")` → `client.startRun` was not called with `chatId`/`switchFromRunId`; `client.sendTurn` was called on `run-old` directly → envelope missing → model forgets `Nam`.
  4. Desktop: while `pendingQuestions = [{questionId:"q1"}]`, `confirmProviderSwitch` still called `switchChatProvider` → `409 handoff_run_busy` + `question_expired`.
  5. Desktop: while `chatDetached=true`, `confirmProviderSwitch` called `switchChatProvider` → `409 chat_no_active_leg` red error instead of defer notice.
- frequency: 100% for detached chats and for switches during pending questions.

## 4. Expected vs Actual

- expected: `sendPrompt` on detached mints a new leg via `startRun(chatId, switchFromRunId)` and the prompt's `TurnRequest` contains the handoff envelope with prior turns; `confirmProviderSwitch` while detached applies `selectedProvider`/`selectedModel` locally with "will reattach on next prompt" and no endpoint call; while a question is pending it shows "Cannot switch provider/model while a question or approval is pending" and does not call the endpoint; `409 chat_no_active_leg` is treated as detached defer, not a red error; `setChatPosture` also blocks when a question is pending.
- actual: `sendPrompt` called `sendTurn` on the terminal runId; `confirmProviderSwitch` called the endpoint in both detached and question-pending cases; `409` surfaced as a red error; `setChatPosture` allowed the switch and cleared the pending question card.

## 5. Impact

- users affected: Desktop users reopening a switched chat after restart or switching provider while a question is pending — the model loses context or the question card is orphaned.
- workflows affected: chat continuity across restarts (Nam memory) and provider switch while the model is awaiting `ask_user`.
- severity: high for chat continuity, medium for the question guard.

## 6. Root Cause

- hypothesis: Desktop never implemented the TUI's `chatDetached` + `cmdReattachChat` pattern.
- confirmed cause: `store.ts` had no `chatDetached` flag; `openHistoryRun` set `runId`/`chatId` but not `chatDetached`; `sendPrompt` had only `if (!runId) startRun` and `else sendTurn`, missing the `if (chatDetached) startRun(chatId, switchFrom)` branch. `confirmProviderSwitch` lacked the `pendingQuestions`/`pendingApprovals` guard and the `chatDetached` defer check before `set({providerSwitchLoading:true})`, and its `catch` had no `chat_no_active_leg` branch. `setChatPosture` had no pending-question guard.
- evidence: `store.ts:1523` `let runId = get().runId; const isFirstChatTurn = !runId` with no detached check; `store.ts:1102` `confirmProviderSwitch` sets `providerSwitchLoading:true` unconditionally before the `try`; `store.ts:1472` `setChatPosture` has no `pendingQuestions` check.

## 7. Fix Strategy

- `F-1` Add `chatDetached?: boolean` to the store state (mirrors TUI), `LAST_CHAT_MODE_KEY` already added for `chat-ui` persist (separate BUG-339 scope) — this BUG covers the detached/question parity.
- `F-2` `openHistoryRun`: derive `isDetached` when `!isWorkflowHistoryItem && Boolean(effectiveChatId) && ["completed","failed","cancelled"].includes(handle.status)` and set `chatDetached: isDetached` alongside `runId`/`chatId`.
- `F-3` `sendPrompt`: introduce `isDetachedReattach = chatMode==="normal_chat" && chatDetached && chatId && runId`; when true, `startRun({ chatId, switchFromRunId: runId, ...current provider/model})` before the turn, clear `chatDetached`, and use the new `runId`/`stepId` for `TurnInput`. Keep the `else if (!runId)` first-turn path verbatim and clear `chatDetached` there too.
- `F-4` `confirmProviderSwitch`: check `pendingQuestions`/`pendingApprovals` first → `providerSwitchLoading:false` + busy system message (no endpoint call, no `pendingProviderSwitch` clear that would orphan the question). Check `chatDetached && chatId` → local `selectedProvider`/`selectedModel` + defer notice + `pendingProviderSwitch:undefined` + `loadSkills` (no endpoint). In `catch`, detect `code==="chat_no_active_leg"` (or message includes it) → same defer, `chatDetached:true`.
- `F-5` `setChatPosture`: check `pendingQuestions`/`pendingApprovals` first → `posture-busy` system message and return (no `getChatPosture`/`setChatPosture`).
- `F-6` `resetRun`, successful switch, and legacy `startRun` paths set `chatDetached:false` so a fresh chat is never detached; `sendPrompt` reattach also clears it.

## 8. Validation

- `V-1` New Desktop tests `store.chat-detached-parity.test.ts`: `openHistoryRun` terminal → `chatDetached:true`; `sendPrompt` on detached → `startRun` called with `chatId`+`switchFromRunId`; `confirmProviderSwitch` while detached → no endpoint call + defer notice; while question pending → busy error + no endpoint; `setChatPosture` while question pending → blocked; `409 chat_no_active_leg` → detached defer; provider-agnostic table over `grok/codex/claude`.
- `V-2` TUI parity: the same `409` string and `Cannot switch` wording as `chat_switch.go:202` + `chatHistory` grouping unchanged.
- `V-3` `npm run typecheck` PASS; existing `store.chatSwitch.test.ts` 6/6 and `store.chat-posture-restore.test.ts` 3/3 remain green (additive only).

## 9. Regression Guard

- tests: `store.chat-detached-parity.test.ts` (6 tests), `store.chat-mode-persist.test.ts` (3 tests) — additive only, no edits to `store.chatSwitch.test.ts`.
- alerts: `chatDetached` must be false after any successful `startRun`/`switchChatProvider`/`reattach`; a question `pendingQuestions.length>0` must never be cleared by a switch attempt.
- audit checks: provider-agnostic (`chatId` only, no `providerKey` branch in `sendPrompt` reattach), additive.

## 10. Follow-Up Document Updates

- upstream docs that must change: `SD-26` §10 detached chats now explicitly mentions Desktop `chatDetached` mirroring TUI `chatDetached` (already implied by F2/F3, now enforced).
- notes left unchanged on purpose: `CA-700` grouping and `CA-699` TUI reattach remain the source of truth for the transcript join.
