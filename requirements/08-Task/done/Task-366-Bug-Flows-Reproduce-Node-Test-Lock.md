# Task-366: Bug Flows Reproduce Node And Coder Test-File Lock

## Metadata

- Document ID: `Task-366`
- Title: `Bug Flows Reproduce Node And Coder Test-File Lock`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-64 P-3](../../07-Coding-Plan/todo/CP-64-Reproduce-First-TDD-Gate.md)
- Child Documents: `None`
- Related Documents: [SD-20](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Replaces: `None`
- Tags: `reproduce-first, bug-harness, flow-topology, frozen-contract, read-only`
- Feature Keys: `reproduce-first-gate`

## AI Quick View

### Summary

- Slice 3 của CP-64: wire quy trình Reproduce-First vào 2 flow fix bug — `bug-harness.yaml` và `bug-plan-harness.yaml` thay node `test_signatures` (signature rỗng) bằng node `reproduce_test` với behavior `agent.reproduce`, prompt `reproduce-failing-test.md`, persona `reproducer.md`, và gate rule `r-reproduce`.
- Thêm cơ chế khóa file test vừa tạo thành read-only cho node `implement` tiếp theo: sau khi reproduce pass, Coder không được phép sửa lại file test để "né lỗi đỏ" — chỉ được sửa production code trong DeclaredPaths.
- Task-harness/rag-harness/vibe-sprint GIỮ NGUYÊN `test_signatures` signature rỗng (CP-64 constraint: không làm gãy flow tính năng mới).

### Current Ask

- Implement P-3 theo CP-64 §4 + §3.2: cập nhật 2 bug flow YAML, edges topology hợp lệ, và khóa ghi file test ở lượt Coder. 3 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Node mới id `reproduce_test` thay thế hẳn node `test_signatures` trong 2 bug flow (không giữ cả hai) — bug fix không còn mode signature rỗng. Edges `context → reproduce_test → implement` (bug-harness) và `preflight_contract_freeze → reproduce_test → implement` (bug-plan-harness) được rename theo.
- `T-2` Khóa read-only thực hiện ở tầng frozen contract: sau turn reproduce pass, đường dẫn file test (từ artifact binding OUTPUT của Task-365) được ghi vào frozen record của step coder như `ReadOnlyPaths`; enforcement tại shared approval bridge `turnBridge.RequestApproval` — cùng nơi Task-340 enforce `posture: read_only` (silent-deny write, mọi provider giống nhau). Frozen contract DeclaredPaths của Coder chỉ chứa production paths. Lock CHỈ được ghi khi flag bật — flag tắt thì không có lock (thuộc legacy fallback T-6).
- `T-3` Topology giữ đúng ràng buộc pack validator hiện tại: đúng MỘT continue back-edge (`validate → implement`); re-entry của reviewer (changes_requested) vẫn resolve về `implement` qua `resolveContinueBackEdgeTarget` — node reproduce chỉ nằm trên chuỗi forward trước implement.
- `T-4` Coder prompt `implement-complete-tests.md` cần một dòng trạng thái mới: file test reproduce đã khóa — chỉ chạy và giữ xanh, không được sửa. (Additive edit nhỏ trong prompt, không tạo prompt mới.)

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-2 shared-logic-per-adapter-wiring — lock enforce tại `turnBridge.RequestApproval` (điểm chung mọi provider); phải đọc 3 callsite Claude/Codex/Grok vào bridge (tiền lệ Task-340 posture read_only) trước khi đóng, không suy diễn.
- Prior CA: none (new feature `reproduce-first-gate`); đóng task phải kèm CA-NNN ledger entry.
- `rag-harness.yaml`, `task-harness.yaml`, `vibe-sprint.yaml` KHÔNG được đổi (diff scope check bắt buộc).
- Flag `FLOWPILOT_ENABLE_REPRODUCE_GATE` tắt → fallback đúng CP-64 §8 về cơ chế signature rỗng cũ (T-6): prompt `test-signatures.md` + agent `tester.md`, không gate, không lock — hành vi bug flow byte-compatible với trước CP-64.
- GitNexus impact analysis trước khi sửa frozen-scope/approval-bridge symbols (Task-340 là d=1 nền).

### Open Questions

- None (timeout reproduce test thuộc Q-1 của Task-364).

### Source Refs

- CP-64 §3.2 (khóa quyền ghi), §4 P-3, test signatures 7–9.
- `internal/agentpack/flow-pack/flows/bug-harness.yaml`, `flows/bug-plan-harness.yaml` (node + edges hiện tại), `internal/changecontract/frozen_scope.go` (DeclaredPaths/AllowedExtraPaths), `internal/agentpack/pack.go` (FlowNode.Posture enforcement note — Task-340).

## 1. Goal

Chạy `/flow bug-harness` (hoặc bug-plan-harness) với một bug thật: sau bước context/freeze, Agent Tester viết test tái hiện ĐỎ qua cổng `r-reproduce`, rồi Coder bước vào `implement` với file test đã khóa ghi — sửa production code đến khi test chuyển XANH, không thể nào sửa test để qua mặt.

## 2. Parent Links

- coding plan: `CP-64-Reproduce-First-TDD-Gate.md` P-3
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`

## 3. Trigger

P-1 (rule engine) và P-2 (prompt/agent/behavior) đã có sẵn; chưa có flow nào sử dụng. Đây là slice đưa Reproduce-First vào production path thực tế của BugFix.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/flows/bug-harness.yaml`**: thay node `test_signatures` bằng node `reproduce_test` (`run: delegate`, `behavior: agent.reproduce`, `agent: agents/reproducer.md`, `promptTemplate: prompts/reproduce-failing-test.md`, artifact binding `file_artifact.v1` OUTPUT cho file test); rename các edge tham chiếu node cũ (`context → reproduce_test`, `reproduce_test → implement`).
- `T-2` **`internal/agentpack/flow-pack/flows/bug-plan-harness.yaml`**: thay đổi tương tự trên chuỗi code phase (sau `preflight_contract_freeze`).
- `T-3` **`internal/changecontract/frozen_scope.go`** + runner bridge: thêm `ReadOnlyPaths []string` vào `FrozenRecord` (zero-value compatible, additive field); helper `isReadOnlyDenied(frozen FrozenRecord, path string) bool` (pure, normalizes slash); enforcement silent-deny tại `turnBridge.RequestApproval` cho write/edit trúng path khóa; runner ghi path file test (artifact binding OUTPUT của node `reproduce_test`) vào frozen record của step `implement` sau khi gate `r-reproduce` pass.
- `T-4` **`internal/agentpack/flow-pack/prompts/implement-complete-tests.md`**: bổ sung đoạn trạng thái reproduce — file test đã khóa, giữ nguyên nội dung, chỉ đưa nó từ ĐỎ về XANH bằng fix production.
- `T-5` **`internal/runner/reproduce_lock_test.go`** (new, không sửa test cũ): 3 test signatures dưới đây (topology test qua pack load; read-only test qua bridge/frozen-scope unit test).
- `T-6` **Legacy fallback runtime theo flag (CP-64 §8)**: khi `FLOWPILOT_ENABLE_REPRODUCE_GATE` tắt, runner degrade node `reproduce_test` về cơ chế signature rỗng cũ TẠI RUNTIME qua `resolveReproducePrompt(flag bool)` — flag tắt resolve prompt về `prompts/test-signatures.md` + agent `agents/tester.md` (khung hàm rỗng, không chạy suite), gate `r-reproduce` không append, không ghi lock; Coder tự điền assertion như trước CP-64. Topology YAML giữ nguyên một node — phân nhánh theo flag diễn ra ở tầng resolve của runner, không phải hai bản flow.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/flows/bug-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/bug-plan-harness.yaml` (modified)
  - `apps/local-runner/internal/changecontract/frozen_scope.go` (modified — ReadOnlyPaths)
  - `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` hoặc bridge file chứa `turnBridge.RequestApproval` wiring (modified — enforcement)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/implement-complete-tests.md` (modified — 1 đoạn additive)
  - `apps/local-runner/internal/runner/reproduce_lock_test.go` (new)
- modules: `agentpack`, `changecontract`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Pack load + validate pass cho cả 2 bug flow với topology mới (node `reproduce_test` present, edges hợp lệ, đúng một continue back-edge).
- [x] AC-2: Node `reproduce_test` mang behavior `agent.reproduce` + prompt `reproduce-failing-test.md` + agent `reproducer.md` + gate rule `r-reproduce` kích hoạt theo P-1.
- [x] AC-3: Sau turn reproduce pass, file test ghi vào frozen record của step `implement` dạng read-only; Coder write/edit trúng path đó bị silent-deny ở bridge.
- [x] AC-4: Frozen contract DeclaredPaths của Coder không chứa file test reproduce (chỉ production paths).
- [x] AC-5: `rag-harness.yaml`/`task-harness.yaml`/`vibe-sprint.yaml` không đổi trong diff.
- [x] AC-6: 3 test signatures green:
  - `TestBugHarnessTopologyContainsReproduceGate`
  - `TestBugPlanHarnessTopologyContainsReproduceGate`
  - `TestCoderNodeHasTestFileAsReadOnly`
- [x] AC-7: Flag tắt → node `reproduce_test` resolve legacy (prompt `test-signatures.md`, agent `tester.md`), không gate, không ghi `ReadOnlyPaths` — Coder sửa/điền file test tự do như trước CP-64 (CP-64 §8 fallback).

## 7. Out of Scope

- Rule engine `r-reproduce` (P-1 / Task-364, đã có).
- Prompt/persona/behavior mới (P-2 / Task-365, đã có).
- E2E lifecycle (P-4 / Task-367).
- Áp dụng reproduce gate cho task mới `behavior-change` trong task-harness/vibe-sprint (Follow-up riêng nếu cần — CP-64 ghi nhận ở mức change_type, không wire flow trong CP này).

## 8. Completion Notes

- result: done 2026-09-16 — `reproduce_test` replaces `test_signatures` in both bug flows (edges renamed, single continue back-edge preserved); `ReadOnlyPaths` on the coder frozen record + silent-deny at `turnBridge.RequestApproval`; `resolveReproducePrompt/Agent` runtime legacy fallback; coder prompt additive paragraph. AC-1→AC-7 ticked; P-3 tests green.
- follow-ups: none — consumed by Task-367.
- upstream docs updated: CA-872 (change-audit/CA-872-CP-64-Reproduce-First-TDD-Gate-Closeout.md).
