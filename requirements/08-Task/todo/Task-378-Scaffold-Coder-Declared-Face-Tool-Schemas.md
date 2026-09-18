# Task-378: Scaffold & Coder Declared Face Tool Schemas

## Metadata

- Document ID: `Task-378`
- Title: `Scaffold & Coder Declared Face Tool Schemas`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: [CP-67 P-1](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [CP-64](../../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [Task-379](./Task-379-Signature-Lock-Rule-AST-Extractor.md)
- Replaces: `None`
- Tags: `contract-first-tdd, tool-face, schema-first, declared-face, flow-control`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice đầu tiên của CP-67: tạo 2 Declared Face Tool Schemas (`submit_scaffold_outcome` và `submit_coder_outcome`) đảm bảo mọi kết quả trả về của TDD và Coder đều là typed JSON, không bao giờ parse prose text tự do (SP-06 Tier 1).
- `submit_scaffold_outcome`: TDD Agent bàn giao danh sách stubs đã tạo, file test suite, danh sách test cases đang ĐỎ, loại lỗi (not_implemented / assertion_failure).
- `submit_coder_outcome`: Coder Agent báo hoàn thành (`completed`), yêu cầu đàm phán lại signature (`renegotiate_signatures` kèm mảng batch requests), hoặc bị nghẽn (`blocked`).
- Slice này chưa wire vào flow nào (P-5 lo); chỉ đảm bảo pack load/validate/render trọn vẹn và schema validation pass.

### Current Ask

- Implement P-1 theo CP-67 §4: 2 file YAML tool face mới, cập nhật manifest, 2 test signatures phải xanh.

### Key Decisions

- `T-1` Cả 2 tool faces dùng `kind: declared_face`, `mapsTo: flow.control`, `exposesTool: true` — tái sử dụng cơ sở hạ tầng `flow_control` primitive hiện có giống hệt `submit-review-outcome.yaml` (không viết handler mới).
- `T-2` `submit_scaffold_outcome` có `statusMap: { scaffold_ready: done, blocked: escalate }`. Payload chứa `stubs` (mảng các symbol khai báo theo file), `test_suite` (file test, danh sách red tests, loại failure). payloadMap forward `stubs` và `test_suite` nguyên dạng vào `payload.*` để hub đọc được structured.
- `T-3` `submit_coder_outcome` có `statusMap: { completed: done, renegotiate_signatures: continue, blocked: escalate }`. Payload `batch_signature_requests` là mảng object (`symbol`, `file`, `current_signature`, `proposed_signature`, `rationale`) — hub dispatch back-edge khi status=continue. payloadMap forward `batch_signature_requests`, `summary`, `implementation_progress` vào payload.
- `T-4` Schema validation: `stubs[].symbols[].kind` enum `[function, method, interface, struct, class]`; `test_suite.failure_type` enum `[not_implemented, assertion_failure]`; `batch_signature_requests[].rationale` required (cấm batch rỗng lý do).

### Constraints

- Không sửa tool face hiện có (`submit-review-outcome.yaml`, `vibe-requirement-outcome.yaml`) — additive only.
- Pack validation phải pass sau khi thêm 2 file mới vào manifest.
- Provider parity: tool face là provider-agnostic (flow.control primitive), không nhánh theo provider.
- Additive tests only — không sửa pre-existing pack tests.

### Open Questions

- None.

### Source Refs

- CP-67 §4 P-1.
- `internal/agentpack/flow-pack/tools/submit-review-outcome.yaml` (mẫu face structure).
- `internal/agentpack/flow-pack/tools/vibe-requirement-outcome.yaml` (mẫu simple face).
- `internal/agentpack/flow-pack/manifest.yaml` (đăng ký).
- `internal/agentpack/pack.go` (ValidateFlowTools parser).

## 1. Goal

Pack flow có đầy đủ 2 Declared Face Tool Schemas cho Contract-First TDD: `submit_scaffold_outcome` (TDD bàn giao stubs + red tests) và `submit_coder_outcome` (Coder báo cáo hoàn thành hoặc nộp batch renegotiate). Pack load/validate pass, schema structure đúng enum/required contract.

## 2. Parent Links

- coding plan: `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md` P-1
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

CP-67 yêu cầu 100% kết quả chuyển giao giữa TDD và Coder phải đi qua typed tool schemas (SP-06 Tier 1). Slice này tạo ra hạ tầng schema mà các slice sau (P-2 gate, P-3 prompt, P-4 prompt, P-5 flow topology) đều tiêu thụ.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/tools/submit-scaffold-outcome.yaml`** (new): Declared face `submit_scaffold_outcome`, kind `declared_face`, mapsTo `flow.control`, exposesTool true. Input schema:
  - `status` (required, enum: `scaffold_ready`, `blocked`)
  - `stubs` (required when scaffold_ready, array of objects: `file` string, `symbols` array of objects: `name` string required, `kind` string enum [function, method, interface, struct, class] required, `signature` string required)
  - `test_suite` (required when scaffold_ready, object: `test_file` string required, `red_tests` array of strings required, `failure_type` string enum [not_implemented, assertion_failure])
  - statusMap: `scaffold_ready: done`, `blocked: escalate`
  - payloadMap: `stubs: payload.stubs`, `test_suite: payload.test_suite`

- `T-2` **`internal/agentpack/flow-pack/tools/submit-coder-outcome.yaml`** (new): Declared face `submit_coder_outcome`, kind `declared_face`, mapsTo `flow.control`, exposesTool true. Input schema:
  - `status` (required, enum: `completed`, `renegotiate_signatures`, `blocked`)
  - `summary` (string, optional)
  - `batch_signature_requests` (array of objects, required when renegotiate_signatures: `symbol` string required, `file` string required, `current_signature` string required, `proposed_signature` string required, `rationale` string required)
  - `implementation_progress` (string, optional)
  - statusMap: `completed: done`, `renegotiate_signatures: continue`, `blocked: escalate`
  - payloadMap: `batch_signature_requests: payload.batch_signature_requests`, `summary: summary`, `implementation_progress: payload.implementation_progress`

- `T-3` **`internal/agentpack/flow-pack/manifest.yaml`** (modified): thêm 2 file vào danh sách `tools`.

- `T-4` **`internal/agentpack/scaffold_tool_face_test.go`** (new): 2 test signatures.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/tools/submit-scaffold-outcome.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/tools/submit-coder-outcome.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (modified)
  - `apps/local-runner/internal/agentpack/scaffold_tool_face_test.go` (new)
- modules: `agentpack`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Pack load + validate pass với manifest mới — 2 tool face files tồn tại, parse thành công.
- [ ] AC-2: `submit_scaffold_outcome` schema đúng contract: `status` enum 2 giá trị, `stubs` là array objects với `symbols[].kind` enum 5 giá trị, `test_suite.failure_type` enum 2 giá trị.
- [ ] AC-3: `submit_coder_outcome` schema đúng contract: `status` enum 3 giá trị, `batch_signature_requests[].rationale` required.
- [ ] AC-4: statusMap mapping đúng: `scaffold_ready→done`, `blocked→escalate`, `completed→done`, `renegotiate_signatures→continue`.
- [ ] AC-5: Tool face hiện có (`submit-review-outcome`, `vibe-requirement-outcome`) không đổi — pre-existing flow không bị ảnh hưởng.
- [ ] AC-6: 2 test signatures green:
  - `TestSubmitScaffoldOutcomeSchemaValidation`
  - `TestSubmitCoderOutcomeBatchSignatureValidation`

## 7. Out of Scope

- Gate rule `r-signature-lock` (P-2 / Task-379).
- Prompt template scaffold-architect (P-3 / Task-380).
- Coder prompt implement-scaffold-body (P-4 / Task-381).
- Wire vào flow topology task-harness/vibe-sprint (P-5 / Task-382).

## 8. Completion Notes

- result: `todo`
- follow-ups: consumed by Task-379/380/381/382.
- upstream docs updated: `todo`
