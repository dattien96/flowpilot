# Task-434 — Modal Routing: Chỉ Focused Run Được Raise Modal

- Document ID: `Task-434`
- Title: `Modal Routing for Non-Focused Runs`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-84 (Realtime Multi-Lane Attention)`
- Child Documents: ``
- Related Documents: `Task-430 (decision payload), Task-431 (inbox controls), Task-429 (events mux)`
- Replaces: ``
- Tags: `attention-queue, desktop-ui, modal-routing`

## AI Quick View

### Summary

- Event từ run nền (quota exhausted, gate, provider-switch prompt, Grok
  yolo posture) hiện set singleton modal state → che chat đang focus.
  Task này route theo `runId` gốc: focused → modal như cũ; non-focused →
  attention item trong inbox.
- Lane B hết quota không bao giờ hijack việc đang gõ ở lane A.

### Current Ask

- Audit mọi code path set `gateBlock` / `pendingAccountSwitch` /
  `pendingProviderSwitch` / `grokYoloPostureLoading`; thêm runId gốc vào
  routing quyết định; non-focused → `ingestAttentionItem` với decision
  payload (Task-430) thay vì modal.

### Key Decisions

- `T-1` Guard helper `isFocusedRun(runId)` — một chỗ quyết định surface,
  các call site không tự nhúng logic so sánh.
- `T-2` Non-focused event tạo attention item qua cùng seam với mux
  (`ingestAttentionItem` push trực tiếp — item synthetic mang đủ
  `decision` payload), không tạo path lẻ.
- `T-3` Action trên item vẫn gọi **cùng client path** với modal (vd
  account-switch confirm) — chỉ khác surface, không khác effect. Quota
  switch vẫn là global per-provider (đúng semantics hiện có), chỉ prompt
  là per-run.
- `T-4` Event đến lúc không có focus (đầu app, chưa select project) →
  coi như non-focused → inbox. Modal không bao giờ pop "trên hư không".

### Constraints

- Focused-run behavior byte-for-byte giữ nguyên — modal hiện có là đúng
  khi event thuộc run đang xem.
- Mọi raise-modal call site phải truyền/resolve được runId nguồn; site
  nào không resolve được runId → mặc định non-focused (fail-safe: inbox,
  không hijack).
- Không đổi quota/account-switch effect — chỉ đổi surface prompt.

### Open Questions

- Event không gắn runId nào (runner-level, vd runner shutdown sắp xảy
  ra)? Lean: giữ modal global — nó *là* global theo nghĩa đen, không
  thuộc lane nào.

### Source Refs

- `CP-84 P-6`, `store.ts` (`gateBlock`, `pendingAccountSwitch`,
  `pendingProviderSwitch`, `grokYoloPostureLoading`, `focusChat`,
  `providerLabel`/`accountLabel`/`findBestCandidate`),
  `GateBlockModal.tsx`, `AccountSwitchModal.tsx`

## 1. Goal

Modal singleton chỉ phục vụ run đang focus; nhu cầu quyết định của lane
nền xếp hàng trong inbox — không bao giờ che màn hình người đang dùng.

## 2. Parent Links

- coding plan: CP-84 (P-6)
- tech design: —
- system spec: SS-13
- specific upstream ids: Task-430, Task-431

## 3. Trigger

Multi-lane triage: `AccountSwitchModal`/`GateBlockModal` là overlay
toàn màn hình — event của run nền pop lên giữa việc gõ của lane khác
(modal hijack). Đây là annoyance trực tiếp của model "1 focus + lane nền".

## 4. Exact Change

- `T-1` `store.ts`: `isFocusedRun(runId)` helper — so với `runId` đang
  focus; `undefined`/không resolve được → false.
- `T-2` Audit + route các set-modal paths: quota/account-switch pending,
  provider-switch prompt, `gateBlock`, `grokYoloPostureLoading`. Mỗi
  site: resolve `runId` nguồn → focused ? modal : `ingestAttentionItem`.
- `T-3` `attentionQueue.ts`: `ingestAttentionItem(item)` — push synthetic
  item (kind từ decision payload; `dispatchAttention`-class dedupe theo
  `runId+kind` nếu đã có).
- `T-4` Resolve path: action từ item gọi cùng store/client function mà
  modal gọi (vd confirm-account-switch) — verify không tạo fork logic.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/state/store.ts`,
  `src/state/attentionQueue.ts`,
  `src/components/AccountSwitchModal.tsx` (đọc — không đổi trừ khi cần
  props), `src/components/GateBlockModal.tsx` (đọc)
- modules: desktop state
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/store.ts
function isFocusedRun(state: AppState, runId: string | undefined): boolean
// runId undefined → false (fail-safe: inbox, không modal)

// mọi call site set modal theo pattern:
const srcRunId = ev.runId /* hoặc runId từ pending record */;
if (isFocusedRun(get(), srcRunId)) {
  set({ pendingAccountSwitch: payload });   // giữ nguyên focused path
} else {
  get().ingestAttentionItem({
    runId: srcRunId, projectId: ev.projectId, kind: "quota",
    decision: ev.decision, /* … */
  });
}
```

```ts
// apps/desktop-flowpilot/src/state/attentionQueue.ts
ingestAttentionItem(item: AttentionItem): void // T-3 — synthetic item push;
// dedupe theo (runId, kind): item đã tồn tại → cập nhật, không nhân đôi
```

## 7. Test Signatures

- `test("non-focused quota event creates inbox item, modal state stays
  null", ...)` — covers D-7 core.
- `test("focused-run quota event still opens AccountSwitchModal", ...)`
  — focused path regression (covers D-7).
- `test("non-focused gate event lands in inbox as gate kind; gateBlock
  untouched", ...)` — covers D-7 gate.
- `test("modal-source event without resolvable runId goes to inbox, no
  modal", ...)` — fail-safe rule (covers T-1 constraint).
- `test("duplicate non-focused quota event dedupes to one item", ...)`
  — covers T-3 dedupe.
- `test("quota item action calls same account-switch confirm path as
  modal", ...)` — covers T-4 no-fork.
- `test("non-focused provider-switch prompt → inbox item", ...)` —
  covers D-7 provider-switch.

## 8. Acceptance Check

- Live: focus lane A đang gõ; lane B hết quota (hoặc hit gate) → inbox
  badge tăng, **không** modal; mở item B → act → lane B unblock; lane A
  không bị gián đoạn.
- Focus lane B lặp lại → modal pop đúng như cũ.

## 9. Out of Scope

- Đổi modal content/design; queue nhiều modal cho cùng focused run
  (giữ singleton semantics); runner-side suppression (client-side
  routing đủ); keyboard shortcut cho modal (CP-84 deferred polish).

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11)
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [ ] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [ ] Provider parity: routing check `runId` thuần, không nhánh provider (R2)
- [ ] `feature_key` = `attention-queue`; CA ledger entry written
- [ ] §8 acceptance checks verified by hand or test
- [ ] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
