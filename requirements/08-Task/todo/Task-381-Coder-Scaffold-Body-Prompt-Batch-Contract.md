# Task-381: Coder Scaffold Body Prompt & Batch Accumulation Contract

## Metadata

- Document ID: `Task-381`
- Title: `Coder Scaffold Body Prompt & Batch Accumulation Contract`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: [CP-67 P-4](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [Task-378](./Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-379](./Task-379-Signature-Lock-Rule-AST-Extractor.md), [Task-380](./Task-380-Scaffold-Architect-Prompt-Agent-Persona.md)
- Replaces: `None`
- Tags: `contract-first-tdd, prompt-template, coder, batch-accumulation, signature-renegotiation`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice 4 của CP-67: tạo prompt `prompts/implement-scaffold-body.md` thay thế `implement-complete-tests.md` trong các flow áp dụng Contract-First TDD — Coder chỉ được phép fill body code, tuyệt đối cấm sửa test (đã khóa ReadOnly) và cấm sửa signature trực tiếp (bị gate `r-signature-lock` chặn).
- Hướng dẫn quy tắc "Accumulate & Batch": khi phát hiện signature cần đổi, Coder KHÔNG dừng lẻ tẻ mà nhớ lại, tiếp tục implement tối đa các phần khác, cuối turn gom toàn bộ thành 1 BATCH REQUEST duy nhất qua `submit_coder_outcome` với status `renegotiate_signatures`.
- Nếu hoàn thành xong, Coder gọi `submit_coder_outcome` với status `completed`.
- Slice này chưa wire vào flow nào (P-5 lo); chỉ bảo đảm prompt render được khi pack load.

### Current Ask

- Implement P-4 theo CP-67 §4: 1 file prompt mới, 2 test signatures phải xanh.

### Key Decisions

- `T-1` Prompt `implement-scaffold-body.md` đối xứng với `implement-complete-tests.md` nhưng khác cơ bản: (a) KHÔNG yêu cầu "fill test signatures" (test đã viết sẵn ĐỎ từ TDD); (b) Yêu cầu Coder chỉ viết logic bên trong `{ }` của các hàm stub; (c) Mô tả rõ quy tắc Accumulate & Batch; (d) Kết thúc turn bằng `submit_coder_outcome`.
- `T-2` Prompt PHẢI ghi rõ 3 lệnh cấm: (a) Cấm sửa file test (đã khóa ReadOnlyPaths); (b) Cấm sửa signature trực tiếp (gate `r-signature-lock` sẽ chặn); (c) Cấm viết test mới (test đã hoàn chỉnh từ TDD, chỉ cần làm cho nó XANH).
- `T-3` Mô tả quy trình batch: nếu phát hiện signature bất cập (ví dụ cần thêm tham số, sai return type) → (a) ghi nhận vào danh sách tạm; (b) tiếp tục code tối đa phần còn lại; (c) nếu có thêm lỗi signature khác tiếp tục gom; (d) cuối turn gọi `submit_coder_outcome` với `renegotiate_signatures` kèm mảng `batch_signature_requests`.
- `T-4` Mục tiêu duy nhất của Coder: "Viết logic nghiệp vụ vào ruột hàm sao cho test suite chuyển từ ĐỎ sang XANH."

### Constraints

- Không sửa `implement-complete-tests.md` — các flow cũ (rag-harness, bug-harness, bug-plan-harness) vẫn dùng prompt cũ.
- Additive only — không sửa pre-existing prompts.
- Prompt phải reference chính xác tên tool `submit_coder_outcome` (from Task-378).

### Open Questions

- None.

### Source Refs

- CP-67 §4 P-4.
- `internal/agentpack/flow-pack/prompts/implement-complete-tests.md` (đối trọng — legacy).
- `internal/agentpack/flow-pack/tools/submit-coder-outcome.yaml` (Task-378 tool schema).

## 1. Goal

Prompt mới `implement-scaffold-body.md` render được trong pack, hướng dẫn Coder: (1) chỉ fill body code stub, (2) không sửa test/signature, (3) gom batch nếu cần đổi signature, (4) kết thúc turn bằng `submit_coder_outcome`.

## 2. Parent Links

- coding plan: `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md` P-4
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

P-1/P-2/P-3 đã có schema, gate rule, và TDD prompt. Slice này tạo prompt tương ứng cho phía Coder để hoàn tất cặp TDD↔Coder contract trước khi P-5 wire vào flow topology.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/prompts/implement-scaffold-body.md`** (new): Hướng dẫn — (1) Bạn đang trong môi trường Contract-First TDD: production stubs và test suite đã sẵn sàng, test đang ĐỎ; (2) Mục tiêu duy nhất: viết logic vào ruột hàm `{ }` sao cho test chuyển ĐỎ→XANH; (3) 3 lệnh cấm: cấm sửa file test (ReadOnly), cấm sửa signature (gate `r-signature-lock`), cấm viết thêm test mới; (4) Khi phát hiện signature cần đổi: KHÔNG DỪNG, nhớ lại, code tiếp, gom thành batch cuối turn; (5) Kết thúc turn: nếu xong → `submit_coder_outcome` status `completed`; nếu cần đổi signature → `submit_coder_outcome` status `renegotiate_signatures` kèm mảng `batch_signature_requests` (mỗi entry: symbol, file, current_signature, proposed_signature, rationale).
- `T-2` **`internal/agentpack/flow-pack/manifest.yaml`** (modified): thêm `prompts/implement-scaffold-body.md` vào `prompts`.
- `T-3` **`internal/agentpack/scaffold_coder_prompt_test.go`** (new): 2 test signatures.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/implement-scaffold-body.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (modified)
  - `apps/local-runner/internal/agentpack/scaffold_coder_prompt_test.go` (new)
- modules: `agentpack`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Prompt template render được khi pack load.
- [ ] AC-2: Prompt chứa 3 lệnh cấm rõ ràng (cấm sửa test, cấm sửa signature, cấm viết test mới).
- [ ] AC-3: Prompt chứa quy trình Accumulate & Batch đầy đủ 4 bước.
- [ ] AC-4: Prompt reference đúng tên tool `submit_coder_outcome` với 2 status (`completed`, `renegotiate_signatures`).
- [ ] AC-5: `implement-complete-tests.md` không đổi.
- [ ] AC-6: 2 test signatures green:
  - `TestImplementScaffoldBodyPromptRendering`
  - `TestImplementScaffoldBodyPromptContainsBatchContract`

## 7. Out of Scope

- Tool face schemas (P-1 / Task-378).
- Gate rule `r-signature-lock` (P-2 / Task-379).
- TDD prompt scaffold-contract-tdd (P-3 / Task-380).
- Flow topology wiring (P-5 / Task-382).

## 8. Completion Notes

- result: `todo`
- follow-ups: consumed by Task-382.
- upstream docs updated: `todo`
