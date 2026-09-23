# Task-431 — Inbox Per-Kind Controls, Preview & Triage

- Document ID: `Task-431`
- Title: `Inbox Per-Kind Controls, Preview & Triage`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-84 (Realtime Multi-Lane Attention)`
- Child Documents: ``
- Related Documents: `Task-423 (inline actions), Task-430 (decision payload), Task-434 (modal routing)`
- Replaces: ``
- Tags: `attention-queue, inbox-triage, desktop-ui`

## AI Quick View

### Summary

- AttentionInbox render control theo `decision.kind` — không còn
  "Approve/Deny cho mọi thứ": gate có radio + custom text (reuse
  GateBlockModal controls), ss_lock có preview AI Quick View,
  worktree_merge có 3 nút choice, quota có "Switch to X".
- Expand `▸` hiện payload inline (tests/outputTail/quickView/
  conflictPaths) mà không rời chat. Filter kind + project; batch-approve
  chỉ eligible kinds.

### Current Ask

- Component `DecisionControls` + `DecisionPreview` trong AttentionInbox;
  store action `submitAttentionDecision` route per-kind tới client calls
  ID-scoped hiện có; triage filter + batch cho eligible kinds.

### Key Decisions

- `T-1` Per-kind control component (không clone GateBlockModal): gate card
  render đúng radio + custom text + [Fix/Suggest/Open] — cùng shape với
  modal để user quen.
- `T-2` Heavy kinds (ss_lock, merge, gate cần đọc) mặc định collapsed +
  expand `▸`; safe kinds (approval/question có options) hiện nút ngay.
  Payload absent → "Open" only (rule hiện có).
- `T-3` Batch chỉ bao `approval` + `question`-with-options — item có
  `decision.kind` ∈ {gate, ss_lock, worktree_merge, quota} luôn
  per-item; checkbox chỉ hiện trên eligible items.
- `T-4` Submit failure 409/stale → toast + refresh histories, item không
  auto-clear (user quyết định retry hay open).

### Constraints

- Submit path ID-scoped: `submitApproval(runId, approvalId, choice)`,
  `answerQuestion(runId, questionId, answer)`, gate/merge/quota tương
  tự — không bao giờ act "run đang focus".
- Không switch focus/project khi inline action thành công; chỉ cập nhật
  inbox + snapshot lane đó.
- Batch là sequential submits với partial-failure report — không phải
  1 bulk endpoint mới.
- Filter state local component (không vào store); default show-all.

### Open Questions

- Batch approve limit? Lean: không cap số item nhưng disable nút khi
  eligible = 0; report từng failure riêng.

### Source Refs

- `CP-84 P-3`, `AttentionInbox.tsx`, `GateBlockModal.tsx`,
  `attentionQueue.ts`, `approveAttentionItem`/`answerAttentionItem`
  (store, Task-423)

## 1. Goal

User triage 5 lane từ inbox mà không rời focused chat: quyết nhanh được
với kind an toàn, đọc đủ context được với kind nặng, filter/batch khi
inbox dài.

## 2. Parent Links

- coding plan: CP-84 (P-3)
- tech design: —
- system spec: SS-13
- specific upstream ids: Task-423, Task-430

## 3. Trigger

CP-84 review: inbox có inline action nhưng thiếu triage — không filter,
không batch, không phân biệt gate an toàn vs cần đọc kỹ; action coverage
chỉ approval/question của focused run.

## 4. Exact Change

- `T-1` `DecisionControls.tsx` (new, trong `components/`): switch trên
  `item.decision.kind`, render per-kind controls; `null` → caller render
  "Open".
- `T-2` `AttentionInbox.tsx`: dùng `DecisionControls`; thêm `▸` expand
  state per item (local), render `DecisionPreview` per kind; header thêm
  kind-filter chips + project filter select + "Approve all eligible" khi
  eligible ≥ 2.
- `T-3` `store.ts`: `submitAttentionDecision` — route theo
  `decision.kind` tới `submitApproval` / `answerQuestion` /
  gate-decision endpoint / merge-choice endpoint /
  account-switch-confirm hiện có; lỗi → `setRunToast` + refresh
  histories (T-4).
- `T-4` `attentionQueue.ts`: `markActing`/`clearActing` (loading state
  per item — hiện có `actingRunIds` chỉ track approve/deny; extend cho
  mọi kind).
- `T-5` Notification deep-link: `notification:show` click →
  `openRunAtAttention(runId)` (đã có plumbing; verify payload runId
  được dùng).

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/AttentionInbox.tsx`,
  `src/components/DecisionControls.tsx` (new), `src/state/store.ts`,
  `src/state/attentionQueue.ts`, `src/styles.css`
- modules: desktop components + state
- routes: none
- tables: none

## 6. Code Guide Signatures

```tsx
// apps/desktop-flowpilot/src/components/DecisionControls.tsx
export function DecisionControls(props: {
  item: AttentionItem;
  acting: boolean;
  onAct: (choice: string, customText?: string) => void;
  onOpen: () => void;
}): JSX.Element | null
// null khi decision absent/kind lạ → caller render nút "Open" (rule hiện có)
```

```tsx
// apps/desktop-flowpilot/src/components/DecisionControls.tsx
export function DecisionPreview(props: { item: AttentionItem }): JSX.Element
// gate: regressedTests list + command + outputTail (collapsible)
// ss_lock: quickView text · worktree_merge: conflictPaths + diffStat
// quota: candidate label + remaining pct · question: options + consequence
```

```ts
// apps/desktop-flowpilot/src/state/store.ts
submitAttentionDecision(runId: string, decision: DecisionPayload, choice: string, customText?: string): Promise<void> // T-3
// routes per decision.kind; ID-scoped; 409/stale → toast + refresh, item giữ nguyên
```

```ts
// apps/desktop-flowpilot/src/state/attentionQueue.ts
markActing(runId: string): void   // T-4 — extend cho mọi kind
clearActing(runId: string): void  // T-4
```

## 7. Test Signatures

- `test("gate item renders radio + custom text + Fix/Suggest/Open", ...)`
  — covers D-3 gate control.
- `test("ss_lock item expand shows quickView; no project switch", ...)`
  — covers D-3 preview-at-distance.
- `test("worktree_merge item shows apply_patch/keep_branch/discard", ...)`
  — covers D-3 merge control.
- `test("quota item 'Switch to X' calls account-switch confirm with
  candidate id", ...)` — covers D-3/D-4 quota routing.
- `test("payload-absent item renders Open only", ...)` — covers D-2 rule
  regression.
- `test("batch approve submits eligible items only, reports partial
  failure", ...)` — covers D-3 batch + T-4.
- `test("kind filter and project filter narrow the list", ...)` — covers
  D-3 triage.
- `test("stale submit (409) shows toast, refreshes histories, keeps
  item", ...)` — covers D-4 error path.
- `test("inline submit does not change selectedProjectId/runId", ...)`
  — covers D-4 no-switch.

## 8. Acceptance Check

- Live: 2 run park ở gate + 1 run chờ approval → từ focused chat của run
  thứ 3: filter "gate", expand xem failing tests, chọn Fix → run unblock
  mà không rời chat; batch-approve item còn lại.

## 9. Out of Scope

- Đổi GateBlockModal focused-run path; keyboard-shortcut navigation
  (CP-84 deferred polish); bulk backend endpoint mới; modal routing
  (Task-434).

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity: controls render từ `decision` payload, không nhánh provider (R2)
- [x] `feature_key` = `attention-queue`; CA ledger entry written
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: DONE — DecisionControls.tsx + DecisionPreview (pure decisionControlModel + renderer), per-kind controls (gate radio+custom / ss_lock quickView / worktree 3 modes / quota Switch-to-X / approval+question chips), expand for heavy kinds, kind+project filters (local state), sequential batch approve for eligible kinds with per-item failure report, submitAttentionDecision ID-scoped routing + markActing/clearActing + runToast on failure + history refresh + item retained, notification deep-link. 9 tests green.
- follow-ups: none.
- upstream docs updated: CP-84, CA-932, FEATURE-KEYS.
