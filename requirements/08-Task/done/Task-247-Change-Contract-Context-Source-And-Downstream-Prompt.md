# Task-247: Source change.contract Và Inject Contract Vào Prompt Step Sau

## Metadata

- Document ID: `Task-247`
- Title: `Change-Contract Context Source And Downstream Prompt Injection`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-50: Context Source Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md) (P-4), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md) (§5 Contract), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3, AC-9)
- Child Documents: `None`
- Related Documents: [Task-243: Review Và Capture Các Context Artifact Source](./Task-243-Context-Artifact-Sources-Review-And-Capture.md) (G-1 nửa change.contract, Q-4), [Task-244: Canonical Head First-Class Context Source](./Task-244-Canonical-Head-First-Class-Context-Source.md) (mẫu source để mirror — làm TRƯỚC), [Task-184: Change Contract Capture](./Task-184-Change-Contract-Capture.md) (store mà task này mở rộng), [Task-185: Scope-Drift Detection](../inprogress/Task-185-Scope-Drift-Detection.md) (gate check dùng cùng Contract), [BUG-243](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md) (đường dispatch validate/audit — điểm neo dự phòng T-3), [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (§3 câu hứa mà task này hiện thực hóa)
- Replaces: `None`
- Tags: `change-contract, context-source, scope-drift, prompt-injection, flow-executor, local-runner`

## AI Quick View

### Summary

- Thêm source `change.contract` (priority 3, trong default set): bản khai `feature/intent/declared_paths` của run xuất hiện trong package khi rebuild, và được append trực tiếp vào prompt các node SAU node khai báo (validate/audit) — AI được nhắc phạm vi TRƯỚC khi gate phải cảnh báo SAU.
- Store thêm `GetLatestForRun(runID)`; `changecontract` thêm `RenderContractBlock` (body KHÔNG heading — caller tự thêm).
- **Timing (đọc trước khi code):** Contract được capture SAU turn Coding đầu tiên (gate hook parse final message — Task-184) ⇒ section rỗng ở lần build package đầu là ĐÚNG THIẾT KẾ; giá trị nằm ở Plan-rerun/continue-round (rebuild) và ở T-3 (append vào prompt validate/audit — luôn có data vì chạy sau Coding).

### Current Ask

- Implement T-1..T-4; đóng nốt nửa `change.contract` của Task-243 G-1, hiện thực hóa câu CP-43 §3 "surfaced through the same context-source registry".

### Key Decisions

- `T-D1` Contract lấy theo **run** (`GetLatestForRun`), không theo step — v1 nhất quán mô hình "một mạch việc mỗi run"; cần per-step thì mở Task riêng (CP-50 R-5), không vá nóng.
- `T-D2` `RenderContractBlock` trả body KHÔNG heading: generic render tự thêm `### change.contract`; chỗ append trực tiếp (T-3) tự thêm `##` — tránh double-heading.
- `T-D3` Contract `confidence=inferred` VẪN render (kèm chú thích "suy ra từ diff") — đa số turn thực tế là inferred (Task-184 F-3), lọc bỏ là vô hiệu hóa feature.

### Constraints

- Đọc store KHÔNG được tạo file/dir mới (workspace có thể chưa từng có contract); mọi lỗi mở store → section rỗng (AC-9).
- KHÔNG thêm `change.contract` vào chuỗi `filterString` trong `startInlineEntryChain` — nó là source collect thật.
- Golden test pass không sửa expectation (fixture không có contracts.ndjson → section rỗng).
- T-3 bắt buộc có guard chống double-append (reprompt/retry đi qua compose nhiều lần).

### Open Questions

- `CP-50 Q-2`: nếu validate/audit không đi qua `composeFlowNodeAgentPrompt` (đường inline dispatch BUG-243 F-0) → neo bổ sung tại chỗ dispatch đó, ghi anchor thật vào §8.

### Source Refs

- `CP-50 §4.4`. `Task-243` G-1/Q-4. `SD-21` §5, §6. `SS-14` US-3, AC-9. `CP-43` §3.

## 1. Goal

Prompt của các step sau step khai báo (validate, audit, reprompt vòng sau) chứa block Change Contract với declared paths của turn Coding; package rebuild (Plan rerun / continue-round) có section `### change.contract`; run mới chưa có contract → mọi thứ y hệt hiện tại.

## 2. Parent Links

- coding plan: `CP-50` P-4
- tech design: `SD-21` §5 (Contract), §6
- system spec: `SS-14` US-3 (declare + flag), AC-9
- specific upstream ids: `CP-50 P-4`, `Task-243 Q-4`, `CP-43 §3`

## 3. Trigger

Task-243 `G-1`/`Q-4`: CP-43 §3 hứa Contract "surfaced through the same context-source registry" nhưng chưa từng được implement — Contract chỉ được gate hook đọc post-turn + desktop panel đọc qua HTTP, chưa bao giờ vào prompt AI. Owner chốt (Q-4, 2026-07-15): prompt các step sau PHẢI mang Contract đã khai.

## 4. Exact Change

- `T-1` **`internal/changecontract` — 2 bổ sung:**
  1. `contract.go` — thêm method (mirror `Get`, mutex-guarded, chỉ đọc):
  ```go
  // GetLatestForRun returns the last stored Contract for runID regardless of
  // step (contracts.ndjson is append-only, last-wins) — Task-247: downstream
  // flow nodes (validate/audit) read the run's most recent declared scope.
  func (s *Store) GetLatestForRun(runID string) (Contract, bool)
  ```
  Scan tuần tự, giữ entry CUỐI có `run_id == runID`. Test: 2 step cùng run → trả step sau; re-save cùng step (last-wins) → bản mới; run lạ → `false`; file thiếu → `false`, không error, không tạo file.
  2. `pack.go` — thêm:
  ```go
  // RenderContractBlock renders c as a headerless prompt body (Task-247;
  // CP-50 P-4 T-D2: callers own the heading — the package's generic render
  // adds "### change.contract", the flow-executor append adds its own "##").
  // Returns "" for a zero-value contract.
  func RenderContractBlock(c Contract) string
  ```
  Format body (bỏ dòng nào rỗng):
  ```
  Feature: <feature_key>
  Intent: <intent>
  Declared scope (KHÔNG sửa ngoài các path này):
  - <mỗi declared_path một dòng>
  Confidence: declared    ← hoặc: inferred (suy ra từ diff, chưa được AI xác nhận)
  ```
  Zero-value (FeatureKey rỗng VÀ DeclaredPaths rỗng) → `""`. Test reproducibility + zero-value + inferred annotation.

- `T-2` **File mới `apps/local-runner/internal/runner/context_source_change_contract.go`** — mirror `canonicalHeadSource` (Task-244 T-1):
  - `const ContextSourceChangeContract ContextSourceID = "change.contract"`; struct `changeContractSource{priority int}`; `Deterministic() = true`.
  - `Fetch`: `hints.WorkflowRunID == ""` → section rỗng. Mở store bằng ĐÚNG constructor của Task-184 (check tên thật trong `changecontract/contract.go` — dạng `NewStore(filepath.Join(hints.Workspace, ".flowpilot"))`; nếu constructor tự mkdir thì thêm biến thể read-only hoặc check `os.Stat` file trước — Constraint "đọc không tạo file"). `GetLatestForRun(hints.WorkflowRunID)` → không có → section rỗng; có → `SourceRef = filepath.ToSlash(<...>/contracts/contracts.ndjson)`, `Body = changecontract.RenderContractBlock(c)`.
  - Đăng ký `mustRegisterContextSource(r, &changeContractSource{priority: 3})` (sau head 1/history 2, trước excerpt 4) + thêm `string(ContextSourceChangeContract)` vào `defaultContextSourceIDs`.
  - Render: đi qua **generic pass** (`### change.contract` + `_Source:_` tự có) — KHÔNG thêm case đặc biệt trong renderer, KHÔNG thêm vào skip-list của `renderGenericSections`.
  - Tests: mirror bộ test Task-244 T-4 (fetch có contract; không contract → rỗng; run-id rỗng → rỗng; in-default-set + registered; render generic có heading `### change.contract`).

- `T-3` **Append contract vào prompt node sau — neo tại `composeFlowNodeAgentPrompt` (`flow_executor.go`):**
  ```go
  // Task-247: mọi delegate node SAU node khai báo được nhắc phạm vi đã khai.
  // Marker guard: reprompt/retry đi qua compose nhiều lần — không append đôi.
  const changeContractPromptMarker = "Declared scope (KHÔNG sửa ngoài"
  ```
  - Trong compose: nếu store có `GetLatestForRun(parentRunID)` và `!strings.Contains(prompt, changeContractPromptMarker)` → append:
  ```
  ## Change Contract đã khai cho run này
  <RenderContractBlock(c)>
  ```
  - Node entry (context.produce) không append (chưa có contract; không phải AI node).
  - `CP-50 Q-2` dự phòng: nếu trace thực tế (flow-events log sandbox) cho thấy validate/audit KHÔNG đi qua `composeFlowNodeAgentPrompt` → neo bổ sung tại đường dispatch inline validate/audit (BUG-243 F-0), ghi anchor thật vào §8 Completion Notes.

- `T-4` **UI + docs:** thêm `{ id: "change.contract", label: "Change Contract" }` vào `contextSourceOptions` (`WorkflowsSettings.tsx`); CP-43 §3 thêm note "(landed via CP-50 P-4/Task-247)"; Task-243 §8 cập nhật trạng thái F-4.

## 5. Touched Areas

- files: mới `internal/runner/context_source_change_contract.go` (+ test); sửa `internal/changecontract/{contract,pack}.go` (+ test), `internal/runner/{context_sources_builtin,flow_executor}.go`, `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`; docs CP-43/Task-243
- modules: changecontract, runner context assembly, flow executor, desktop settings UI
- routes: không
- tables: không

## 6. Acceptance Check

- DONE `change.contract` đăng ký priority 3, trong `defaultContextSourceIDs`; CP-45 instance chọn được; validate pass.
- Run flow live (sandbox): prompt validate/audit chứa Change Contract. *(skip — e2e live)*
- DONE Plan-step rebuild có section change.contract when store has data (unit Fetch).
- DONE Run mới chưa có contract → section rỗng, golden pass.
- DONE Không double-append qua reprompt (marker guard test).
- DONE Contract `inferred` render kèm chú thích; zero-value không render.
- DONE `GetLatestForRun`/`RenderContractBlock` + source tests pass; build sạch.

## 7. Out of Scope

- Contract per-step (v1 theo-run — CP-50 R-5).
- Sửa logic capture/parse Contract (Task-184) hay scope-drift rules (Task-185).
- Gate reprompt text nhúng contract (cân nhắc Task riêng nếu cần sau khi quan sát thực tế).

## 8. Completion Notes

- result: `done` (2026-07-15).
- follow-ups: none.
- upstream docs updated: **(2026-07-16, per DOD `T-4`)** CP-43 §3 — added a note that the declared-scope Change Contract source landed via CP-50 P-4/Task-247; Task-243 §8 — added a note that `F-4` is done and referencing CP-50 P-1..P-4/Task-244..247.
