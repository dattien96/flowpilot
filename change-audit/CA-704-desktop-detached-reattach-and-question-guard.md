# CA-704 — Desktop detached reattach, 409 defer, and question-pending guard (BUG-340)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-340
change_type: bugfix
summary: Desktop sendPrompt reattaches detached chats via startRun(chatId, switchFromRunId), confirmProviderSwitch defers when detached and blocks when a question is pending, and 409 chat_no_active_leg is treated as detached defer
# --->8---

## What changed

- `store.ts` state: add `chatDetached?: boolean` (mirrors TUI `chatDetached`), derived in `openHistoryRun` from `handle.status` terminal (`completed`/`failed`/`cancelled`) when `chatId` exists and `runKind` is chat.
- `sendPrompt`: introduce `isDetachedReattach = chatMode==="normal_chat" && chatDetached && chatId && runId`; when true, `startRun({ chatId, switchFromRunId: runId, ...current provider/model })` before the turn, clear `chatDetached`, and use the new `runId`/`stepId` for `TurnInput`. Keep `else if (!runId)` first-turn path verbatim and clear `chatDetached` there too.
- `confirmProviderSwitch`: check `pendingQuestions`/`pendingApprovals` first → `providerSwitchLoading:false` + busy system message (no endpoint call); check `chatDetached && chatId` → local `selectedProvider`/`selectedModel` + defer notice + `loadSkills` (no endpoint); in `catch`, detect `code==="chat_no_active_leg"` (or message includes it) → same defer, `chatDetached:true`.
- `setChatPosture`: check `pendingQuestions`/`pendingApprovals` first → `posture-busy` system message and return (no `getChatPosture`/`setChatPosture`).
- `resetRun`, successful `switchChatProvider`, legacy `startRun`, and reattach all set `chatDetached:false`.

## R1 evidence

- New tests `store.chat-detached-parity.test.ts` (6 tests): `openHistoryRun` terminal → `chatDetached:true`; `sendPrompt` on detached → `startRun` called with `chatId`+`switchFromRunId`; `confirmProviderSwitch` while detached → no endpoint call + defer notice; while question pending → busy error + no endpoint; `setChatPosture` while question pending → blocked; `409` → detached defer; provider-agnostic table over `grok/codex/claude` (chatId only).
- `npm run typecheck` PASS; existing `store.chatSwitch.test.ts` 6/6 and `store.chat-posture-restore.test.ts` 3/3 remain green (additive only).

## Honest gaps

- None; F4 `/open` backfill was already Desktop-done (`buildPriorChatTimeline`), not re-tested here.
