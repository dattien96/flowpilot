# BUG-356: cp-harness slice-only audit kẹt blocked_validation_failed không lối thoát (thiết kế không có validate node)

## Metadata

- Document ID: `BUG-356`
- Title: `cp-harness slice-only audit kẹt blocked_validation_failed không lối thoát`
- Phase: `bugfix`
- Status: `open`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-05`
- Last Updated: `2026-09-05`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [CP-58-Test-Steps](../../07-Coding-Plan/inprogress/CP-58-Test-Steps.md), [Task-306](../../08-Task/inprogress/Task-306-Cp-Harness-12-Step-With-Cp-Plan-And-Task-Splitter.md)
- Child Documents: `none`
- Related Documents: [BUG-354](../done/BUG-354-Gate-Oracle-Hang-Implement-Running-Forever.md) (cùng session CP-58 live)
- Replaces: `none`
- Tags: `agent-flow-engine, cp-harness, slice-only, audit, fail-closed, validation, live-2026-09-05`

## AI Quick View

### Summary

- Symptom (live 2026-09-05, run-556588, flow `cp-harness` slice-only): `task_splitter` DONE → `audit` RUNNING → park `WAITING_USER_APPROVAL` tại `cp_synthesis` với reason `Audit blocked: validation was not positively verified (status=blocked_validation_failed, validation=)`.
- Expected (Task-306 T-2 + CP-58 C5): slice-only đi `task_splitter → audit → done`, không coding chain.
- Actual: audit không bao giờ pass được — `BuildAuditDraft` đòi `ValidationResult == "passed"` (`flow_audit_draft.go:101-104`), mà `flowValidationRetryState` chỉ được set bởi validate node (`flow_validate_audit_dispatch.go:625/720`) hoặc resume — slice-only theo thiết kế KHÔNG có validate node (Task-306 T-1 cấm). Park này không có lối thoát thuận: Retry/Continue chỉ re-run audit node với cùng state rỗng → block lại y hệt; chỉ còn Stop (bỏ run).
- Impact: flow mặc định của Task-306 (deliverable chính) không thể `done` — mọi slice-only run kết thúc bằng park. CP-58 C5 FAIL vì giới hạn sản phẩm, không phải do test sai.

### Current Ask

- Quyết thiết kế: slice-only audit đi đường nào — (a) docs-only fast path (waive/khác chuẩn verify khi aggregate diff chỉ là docs), (b) inline validation nhẹ cho splitter output (file tồn tại + traceability, không `go test`), hay (c) operator override tường minh kiểu SS-17 (yếu fail-closed, cần thiết kế cẩn thận).
- Capture-only ở bước này: chưa sửa code, chưa thêm test.

### Key Decisions

- D-1: Đánh FAIL C5 và file bug thay vì bấm Retry mù (Retry đã chứng minh futile bằng đọc code: state rỗng → block lại).
- D-2: Không sửa Task-306 hay CP-58 C5 trong lúc này — chờ quyết thiết kế rồi cập nhật upstream cho đúng.

### Constraints

- additive-tests-only / oracle-rule như mọi fix trong repo.
- V9-01 fail-closed (không finalize khi chưa verify) là chủ ý — fix không được mở fail-open lén.

### Open Questions

- Q-1: `skipped_no_command` ở validate đã escalate xin human config/continue — nhưng audit sau đó vẫn block. Lộ trình "human verify tay rồi Continue" hiện gãy ở audit cho mọi flow không validate. Đây có phải cùng 1 bug hay 2 mặt của 1 thiết kế thiếu?
- Q-2: cp-harness-smoke (13 nodes, có validate) có pass audit không — cần 1 run D để đối chứng (nếu smoke pass → càng khẳng định slice-only thiếu đường verify riêng).

### Source Refs

- `apps/local-runner/internal/runner/flow_audit_draft.go:98-117` — `ValidationResult != "passed"` → block; `featureKeyRegistered` sau đó.
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:998-1021` — `runAuditNode` đọc `rs.flowValidationRetryState` (nil khi không validate node).
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:604-645` — state chỉ sinh ở validate node.
- `requirements/08-Task/inprogress/Task-306-Cp-Harness-12-Step-With-Cp-Plan-And-Task-Splitter.md:23,33` (T-2 slice-only), `:75` (T-1 cấm coding chain).
- Live: run-556588 `task_splitter` DONE → audit park; splitter đã sinh đúng 3 Tasks (910/911/912 ↔ P-1/P-2/P-3, Parent Documents trỏ CP-38, additive hoàn toàn).

## Evidence

- Screenshot operator 2026-09-05: `cp_synthesis WAITING_USER_APPROVAL`, reason `Audit blocked: validation was not positively verified (status=blocked_validation_failed, validation=)`; chips `[Retry]/[Stop]`.
- run-556588: C1 PASS (1 `[agent-spawn]` entry `contract-planner`); C2 PASS (`CP-38-abs-benchmark.md` đủ §1–§8 + P-1/P-2/P-3 + §7 Validation Plan); C4 PASS (3 Task files đúng N=3, traceability P-*, zero file sửa — `git status` chỉ `??` + ledger bookkeeping).
- Đọc code xác nhận Retry futile: `runAuditNode` không sinh validation state, chỉ đọc → re-run cho cùng kết quả.

## Root Cause

Thiết kế (Task-306: slice-only không validate) xung đột với gate (audit đòi validation passed). Không bên nào sai một mình — thiếu quyết định "slice-only verify bằng gì". Fail-closed làm đúng việc của nó (không finalize bừa); sai là không có đường verify hợp lệ cho flow docs-only.

## Fix direction (proposed, NOT implemented)

- Ưu tiên (b): inline validation nhẹ sau splitter (tồn tại file + traceability Parent→CP/P-* + additive check) rồi set validation passed cho audit — vừa giữ fail-closed vừa cho slice-only lối thoát.
- (a) docs-only waive: nhanh nhưng định nghĩa "docs-only" phải chặt (aggregate diff toàn `.md` trong `requirements/`?).
- (c) operator override: cuối cùng mới đụng (yếu gate).
- Validation khi fix: unit mới (slice-only audit path) + rerun live run-556588-kiểu tới `done`.
