# Task-365: Reproducer Prompt, Agent Persona, And Behavior

## Metadata

- Document ID: `Task-365`
- Title: `Reproducer Prompt, Agent Persona, And Behavior`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-64 P-2](../../07-Coding-Plan/todo/CP-64-Reproduce-First-TDD-Gate.md)
- Child Documents: `None`
- Related Documents: [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- Replaces: `None`
- Tags: `reproduce-first, prompt-template, agent-pack, behavior-registry, tdd`
- Feature Keys: `reproduce-first-gate`

## AI Quick View

### Summary

- Slice 2 của CP-64: tạo prompt template `prompts/reproduce-failing-test.md` thay thế tư duy "chỉ viết khung hàm rỗng" của `test-signatures.md` trong bối cảnh bug fix — Tester phải viết test HOÀN CHỈNH với assertion thật, chạy ra ĐỎ vì assertion, và tuyệt đối không chạm production code.
- Tạo agent persona `agents/reproducer.md` chuyên tái hiện bug, và đăng ký behavior mới `agent.reproduce` vào runtime registry + reference registry + manifest của flow-pack.
- Node reproduce khai báo file_artifact OUTPUT binding ghi lại đường dẫn file test tái hiện — đây là đầu vào cho cơ chế khóa read-only của P-3 (Task-366).
- Slice này chưa wire vào flow nào (P-3 lo); chỉ bảo đảm pack load/validate/render được trọn vẹn.

### Current Ask

- Implement P-2 theo CP-64 §4: 2 file mới (`reproduce-failing-test.md`, `reproducer.md`), đăng ký behavior `agent.reproduce`, cập nhật `manifest.yaml`, 2 test signatures phải xanh.

### Key Decisions

- `T-1` Prompt `reproduce-failing-test.md` đối xứng ngược với `test-signatures.md`: nơi test-signatures cấm viết test body + cấm chạy suite, reproduce-failing-test BẮT BUỘC chạy test và phải chứng minh output đỏ trước khi kết thúc turn; cả hai cùng cấm chạm production code và cấm sửa test cũ.
- `T-2` Behavior `agent.reproduce` scope `delegate`, runtime handler map onto `behaviorAgentDelegate` (spawn provider-backed child như `agent.code`) nhưng với write-scope khép kín: DeclaredPaths chỉ chứa file test tái hiện mới — production write bị frozen contract deny sẵn. Không viết handler spawn mới.
- `T-3` Runtime source of truth là `runner/behavior_registry_builtin.go`; entry trong `behaviors/registry.yaml` là reference-only documentation (file này không được pack parser load) — phải sửa CẢ HAI, đúng như note của registry.yaml.
- `T-4` Node reproduce dùng artifact binding `file_artifact.v1` OUTPUT để ghi nhận đường dẫn file test bắt buộc tồn tại sau turn (tái sử dụng `r-artifact-output` Task-223 thay vì viết rule tồn-tại mới).

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — prompt/persona/behavior `agent.reproduce` tái dùng `behaviorAgentDelegate` nguyên vẹn, không nhánh `providerKey` (grep verify 0 hit); 1 representative test đủ.
- Prior CA: none (new feature `reproduce-first-gate`); đóng task phải kèm CA-NNN ledger entry.
- Không sửa `test-signatures.md` hay `agents/tester.md` — task mới dùng signature rỗng như cũ (CP-64: tránh kẹt compile error ở tính năng mới).
- `manifest.yaml` phải khai báo file mới, nếu không pack load sẽ fail validation.
- GitNexus impact analysis trước khi sửa symbol trong `behavior_registry_builtin.go`.

### Open Questions

- None.

### Source Refs

- CP-64 §3.1, §4 P-2, test signatures 5–6.
- `internal/agentpack/flow-pack/prompts/test-signatures.md` (đối trọng), `internal/agentpack/flow-pack/manifest.yaml` (đăng ký agents/prompts), `internal/agentpack/flow-pack/behaviors/registry.yaml` (reference registry), `internal/runner/behavior_registry_builtin.go` (runtime registry).

## 1. Goal

Pack flow có đầy đủ 3 mảnh của node reproduce: prompt template hướng dẫn viết test đỏ có assertion thật, persona reproducer chuyên biệt, behavior `agent.reproduce` đăng ký runtime — pack load/validate/render thành công và binding file test được ghi nhận.

## 2. Parent Links

- coding plan: `CP-64-Reproduce-First-TDD-Gate.md` P-2
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

P-1 (Task-364) đã có rule engine; nhưng rule cần node có behavior `agent.reproduce` để kích hoạt (CP-64 P-1 activation condition). Slice này sinh ra các mảnh pack đó trước khi P-3 wire vào bug flow.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/prompts/reproduce-failing-test.md`** (new): hướng dẫn — (1) đọc bug report/stacktrace từ frozen contract; (2) viết đúng MỘT file test mới tái hiện bug với assertion đầy đủ (`expected X got Y`), không assertion rỗng; (3) CẤM sửa production code, CẤM sửa/weaken test cũ; (4) chạy test suite và kết thúc turn chỉ khi test ĐỎ vì assertion — test compile-lỗi hoặc chạy Xanh đều là turn thất bại và sẽ bị gate reprompt.
- `T-2` **`internal/agentpack/flow-pack/agents/reproducer.md`** (new): persona chuyên tái hiện bug — kỹ sư test hạ tầng, tư duy "bug chưa tái hiện = bug chưa tồn tại", không tối ưu cho test pass.
- `T-3` **`internal/agentpack/flow-pack/manifest.yaml`**: thêm `agents/reproducer.md` vào danh sách `agents` và `prompts/reproduce-failing-test.md` vào danh sách `prompts`.
- `T-4` **`internal/runner/behavior_registry_builtin.go`** + **`internal/agentpack/flow-pack/behaviors/registry.yaml`**: đăng ký behavior `agent.reproduce` (scope `delegate`, aliases `reproduce`/`reproducing_test`, purpose tái hiện bug bằng test đỏ có assertion, runtimeHandler `behaviorAgentDelegate`, write-scope chỉ file test mới).
- `T-5` **`internal/agentpack/reproduce_behavior_test.go`** (new, không sửa pack test hiện có): 2 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/reproduce-failing-test.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/agents/reproducer.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/behaviors/registry.yaml` (modified — reference doc)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (modified — runtime registration)
  - `apps/local-runner/internal/agentpack/reproduce_behavior_test.go` (new)
- modules: `agentpack`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Prompt template render được khi pack load (không lỗi parse/template).
- [x] AC-2: Pack validation pass với manifest mới — agent file + prompt file tồn tại, behavior `agent.reproduce` resolve được ở runtime registry.
- [x] AC-3: Behavior registry (runtime + reference) mô tả đúng contract: delegate spawn, write-scope chỉ file test mới, production write bị deny.
- [x] AC-4: `test-signatures.md` và `agents/tester.md` không đổi — task-harness/rag-harness/vibe-sprint không bị ảnh hưởng.
- [x] AC-5: 2 test signatures green:
  - `TestReproducerAgentPromptRender`
  - `TestReproducerArtifactBinding`

## 7. Out of Scope

- Wire node vào `bug-harness.yaml`/`bug-plan-harness.yaml` (P-3 / Task-366).
- Khóa read-only file test cho coder (P-3 / Task-366).
- Gate rule `r-reproduce` (đã thuộc P-1 / Task-364).
- E2E lifecycle (P-4 / Task-367).

## 8. Completion Notes

- result: done 2026-09-16 — `reproduce-failing-test.md` + `reproducer.md` registered in `manifest.yaml`; `agent.reproduce` in runtime (`behavior_registry_builtin.go`, handler `behaviorAgentDelegate`) + reference (`behaviors/registry.yaml`); file_artifact OUTPUT binding carries the test path to P-3. AC-1→AC-5 ticked; P-2 tests green.
- follow-ups: none — consumed by Task-366/367.
- upstream docs updated: CA-872 (change-audit/CA-872-CP-64-Reproduce-First-TDD-Gate-Closeout.md).
