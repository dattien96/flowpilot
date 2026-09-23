# Task-432 — Per-Chat Prompt Drafts

- Document ID: `Task-432`
- Title: `Per-Chat Prompt Drafts`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-84 (Realtime Multi-Lane Attention)`
- Child Documents: ``
- Related Documents: `CP-59 (chat SSOT / legs), Task-421 (project-scoped bootstrap)`
- Replaces: ``
- Tags: `chat-drafts, desktop-ui, ux`

## AI Quick View

### Summary

- Draft prompt key theo `chatId` (fallback `runId`, `"<projectId>:new"`
  cho new chat) trong store map + persist `localStorage`. Switch project/
  run/chat không còn mất text đang gõ.
- Composer vẫn singleton — không đụng focused-run model; draft là
  client-local, không bao giờ lên Drive/Supabase.

### Current Ask

- `drafts.ts` helper (key derivation, load/save, LRU cap) + store field/
  actions + `ChatInput` lift draft state lên store. Clear đúng key khi
  send; prune khi chat bị xoá.

### Key Decisions

- `T-1` Key = `chatId` vì provider/model switch tạo runId mới nhưng draft
  thuộc chat logic; workflow-run không có chatId → `runId`; new chat →
  `"<projectId>:new"` (draft đầu tiên clear khi chat minted bằng send).
- `T-2` Persist `localStorage` (pattern `LAST_PROJECT_KEY`/`workingMode`
  hiện có) — cap 50 entries, prune oldest `updatedAt`; đây là device-local
  intent, không sync.
- `T-3` Draft chứa text + mentions + attachment refs — serialize được
  (không giữ object phức tạp); mention/attachment shape reuse type hiện
  có của ChatInput.

### Constraints

- `resetRun()`/`selectProject` không đụng `drafts` — draft sống sót mọi
  navigation.
- Clear key chỉ sau khi send **thành công** (send lỗi giữ draft).
- Không đổi `sendPrompt` signature — chỉ thêm clear-draft ở success path.
- Chat bị delete → prune keys của nó (listener đơn giản trong delete flow).

### Open Questions

- Composer rỗng vs draft rỗng: entry rỗng (`text=""`, không attachments)
  không persist — giữ map gọn.

### Source Refs

- `CP-84 P-4`, `ChatInput.tsx`, `store.ts` (`chatId`/`runId`/
  `selectProject`/`resetRun`, `LAST_PROJECT_KEY` pattern)

## 1. Goal

Gõ dở ở lane A → xử lý noti lane B → quay lại A: text/mentions/
attachments còn nguyên, kể cả qua app reload.

## 2. Parent Links

- coding plan: CP-84 (P-4)
- tech design: —
- system spec: SS-13
- specific upstream ids: CP-59

## 3. Trigger

`ChatInput` draft hiện nằm trong component state; `selectProject` →
`resetRun()` wipe. Multi-lane triage làm switch thường xuyên → mất draft
là annoyance hàng ngày.

## 4. Exact Change

- `T-1` `src/state/drafts.ts` (new): `DraftState`, `draftKeyFor`,
  `loadDrafts`, `saveDrafts`, `DRAFT_CAP = 50`, prune helper.
- `T-2` `store.ts`: `drafts: Record<string, DraftState>` (init từ
  `loadDrafts()`), `setDraft(key, draft)`, `clearDraft(key)`; persist
  debounced ~300ms hoặc on-unmount flush.
- `T-3` `ChatInput.tsx`: lift `prompt`/mentions/attachments lên
  `drafts[draftKeyFor(chatId, runId, selectedProjectId)]`; giữ disabled/
  focus behavior hiện có.
- `T-4` Send path: sau `sendPrompt` resolve → `clearDraft(key)`; delete
  chat flow → prune key.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/state/drafts.ts` (new),
  `src/state/store.ts`, `src/components/ChatInput.tsx`
- modules: desktop state + component
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/drafts.ts
export interface DraftState {
  text: string;
  mentions?: FileMentionRef[];      // reuse ChatInput mention type
  attachments?: AttachmentRef[];    // reuse ChatInput attachment type
  updatedAt: number;
}
export const DRAFT_CAP = 50;

export function draftKeyFor(chatId: string | null, runId: string | null, projectId: string | null): string
// chatId ?? runId ?? `${projectId}:new`; projectId null → "global:new"

export function loadDrafts(): Record<string, DraftState>                    // localStorage, corrupt → {}
export function saveDrafts(drafts: Record<string, DraftState>): void        // prune cap trước khi ghi
export function pruneDrafts(drafts: Record<string, DraftState>, keep: Set<string>): Record<string, DraftState>
```

```ts
// apps/desktop-flowpilot/src/state/store.ts
drafts: Record<string, DraftState>;                        // init loadDrafts()
setDraft(key: string, draft: DraftState): void;            // skip persist khi empty
clearDraft(key: string): void;
```

```tsx
// apps/desktop-flowpilot/src/components/ChatInput.tsx
const draftKey = draftKeyFor(chatId, runId, selectedProjectId);
const draft = useStore((s) => s.drafts[draftKey]);          // value source
// onChange → s.setDraft(draftKey, {...}); send success → s.clearDraft(draftKey)
```

## 7. Test Signatures

- `test("draft survives selectProject + resetRun", ...)` — covers D-5
  core: gõ ở A, select B, back → text còn.
- `test("send success clears only that chat's draft", ...)` — draft lane
  khác nguyên vẹn (covers D-5).
- `test("send failure keeps draft", ...)` — error path.
- `test("drafts persist to localStorage and reload on store init", ...)`
  — covers D-5 reload.
- `test("new-chat draft keyed '<projectId>:new' survives project switch",
  ...)` — covers T-1 fallback key.
- `test("deleting a chat prunes its draft keys", ...)` — covers T-4.
- `test("draft map prunes oldest beyond DRAFT_CAP", ...)` — covers T-2.
- `test("empty draft is not persisted", ...)` — covers OQ resolution.

## 8. Acceptance Check

- Live: gõ dở ở project A → switch B, quay lại → draft còn. Gửi →
  composer trống, draft lane B vẫn giữ. Restart app → drafts còn.

## 9. Out of Scope

- Multiple simultaneous composers / per-run interactive state (CP-84
  R-5 non-goal); sync draft lên Drive/Supabase (explicit non-goal);
  draft cho GateBlockModal custom text (modal per-run, Task-434 scope
  khác).

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity N/A — pure client-side state (noted in §11)
- [x] `feature_key` = `chat-drafts` (append vào FEATURE-KEYS.md); CA entry written
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: DONE — drafts.ts (draftKeyFor chatId>runId>project:new>global:new, localStorage persist, DRAFT_CAP=50 LRU by updatedAt, empty never persisted), store setDraft/clearDraft debounced, sendPrompt clears-on-success/restores-on-failure, ChatInput hydrates+mirrors, deleteHistoryRun prunes. 10 tests green.
- follow-ups: deviation — DraftState reuses the composer's actual SkillToken/PendingAttachment shapes instead of spec's FileMentionRef/AttachmentRef (singleton composer; a parallel mention type would be dead weight).
- upstream docs updated: CP-84, CA-933, FEATURE-KEYS (chat-drafts).
