# CA-933 — Per-chat prompt drafts (CP-84 Task-432)

## Summary

Switching chats wiped the composer. Drafts are now keyed by logical chat
identity — `chatId` first, `runId` fallback, `${projectId}:new` for a new
chat, `global:new` without a project — and persist client-local in
localStorage (cap 50, oldest pruned by `updatedAt`; empty drafts never
persisted; never uploaded anywhere).

- `state/drafts.ts`: `DraftState{text, selectedSkills, attachments, updatedAt}`,
  `draftKeyFor`, `loadDrafts`/`saveDrafts`, `pruneDrafts`, `DRAFT_CAP=50`.
- Store: `drafts` map + `setDraft`/`clearDraft` with 300ms debounced persist;
  `sendPrompt` captures the key pre-send, clears on success, restores on
  failure only when the user hasn't typed a newer draft; `deleteHistoryRun`
  prunes run+chat keys.
- `ChatInput` hydrates composer state on key change and mirrors edits back.

Deviation (documented in Task-432 §11): draft state reuses the composer's
actual `SkillToken`/`PendingAttachment` shapes rather than introducing the
spec's `FileMentionRef`/`AttachmentRef` — the composer is a singleton, so a
second mention type would have been dead weight.

## Verified

- 10 new tests green (`drafts.test.ts`, `localStorageTestStub.ts`).

## Files

- `state/drafts.ts` (new), `state/drafts.test.ts` (new),
  `state/localStorageTestStub.ts` (new), `state/store.ts`,
  `components/ChatInput.tsx`

# ---8<--- flowpilot:change-ledger
feature_key: chat-drafts
source_doc_id: CP-84
change_type: feature
summary: per-chat composer drafts keyed by chatId/runId/project:new, localStorage-persisted, capped LRU, cleared only on successful send
# --->8---
