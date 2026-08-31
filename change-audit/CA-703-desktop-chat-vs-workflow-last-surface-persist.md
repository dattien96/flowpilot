# CA-703 — Desktop Chat vs Workflow last-surface persist (BUG-339 follow-up)

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-339
change_type: bugfix
summary: persist last Chat vs Workflow surface to localStorage and restore on loadProjects when no active run
# --->8---

## What changed

- `store.ts` add `LAST_CHAT_MODE_KEY = "fp:lastChatMode"` (mirrors `fp:lastProjectId`).
- `setChatMode(mode)`: `localStorage.setItem(LAST_CHAT_MODE_KEY, mode)` before `set({chatMode})` + `resetRun()`.
- `loadProjects` boot: after `listProjects`/`selectProject`, restore `chatMode` from `localStorage` when `!get().runId` and the stored value is `normal_chat` or `workflow_step_auto` (no override when a history item is being opened — `openHistoryRun` still sets `chatMode` from `historyItem.runKind` per BUG-170).
- `resetRun` clears `chatDetached` (orthogonal, but added here for completeness).

## R1 evidence

- New tests `store.chat-mode-persist.test.ts` (3 tests): `setChatMode` writes `localStorage`; `loadProjects` restores `workflow_step_auto` when no active run; `openHistoryRun` restores `chatMode` from history item, not from persisted last surface.
- `npm run typecheck` PASS; existing `store.chat-posture-restore.test.ts` 3/3 remain green (additive only).

## Honest gaps

- None; detached reattach + question guard are `CA-704`.
