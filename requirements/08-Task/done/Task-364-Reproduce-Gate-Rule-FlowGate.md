# Task-364: Reproduce Gate Rule In FlowGate Engine

## Metadata

- Document ID: `Task-364`
- Title: `Reproduce Gate Rule In FlowGate Engine`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-64 P-1](../../07-Coding-Plan/todo/CP-64-Reproduce-First-TDD-Gate.md)
- Child Documents: `None`
- Related Documents: [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-20](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- Replaces: `None`
- Tags: `flowgate, reproduce-first, oracle-rule, gate-hook, red-green`
- Feature Keys: `reproduce-first-gate`

## AI Quick View

### Summary

- Slice đầu tiên của CP-64: định nghĩa rule gate mới `r-reproduce` trong `flowgate` — cổng ép turn Tester phải tạo ra ÍT NHẤT 1 test ĐỎ (assertion failure) trước khi được phép sửa production code trong BugFix.
- Phân biệt tuyệt đối 2 loại thất bại: Compile Error (KHÔNG tính là reproduce — chặn, reprompt sửa cú pháp) vs Assertion Failure (ĐẠT chuẩn reproduce — pass gate).
- Rule KHÔNG nằm trong `DefaultRules()` (giữ `TestDefaultRules` byte-stable, pattern của `r-requirement`): gate hook chỉ append khi node active là reproduce (behavior `agent.reproduce`) hoặc turn mang `change_type: behavior-change` trong flow fix bug.
- Tại node reproduce, `r-tests`/`r-reg` phải được suppress đúng 1 turn (test sinh ra LÀ ĐỂ fail) — precedent: proposal-turn exemption đã có trong `gate_hook.go`.

### Current Ask

- Implement P-1 theo CP-64 §4: `reproduce_rule.go` (new) + mở rộng phân loại oracle + wiring trong `runFlowGateAtEpoch`. 4 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Rule `r-reproduce` theo pattern `r_requirement.go`: builder riêng, không đưa vào `DefaultRules()`, được append có chọn lọc theo ngữ cảnh node/flow (CP-64 P-1: chỉ kích hoạt trên node `agent.reproduce` hoặc node `test_signatures` của bug flow; không chặn flow task tính năng mới).
- `T-2` Trigger mới `reproduce_not_demonstrated`, Action `reprompt` — cả 2 nhánh fail (compile error, all-pass) đều reprompt với Detail khác nhau; nhánh assertion-failure → KHÔNG violation (pass gate).
- `T-3` Phân loại compile vs assertion chạy trên `OracleResult.Output` hiện có (đã được capture full stdout+stderr): compile error nhận dạng qua signature của từng runner format (`go test`: `# pkg`, `syntax error`, `undefined:`, `[build failed]`; `npm`: `SyntaxError`/error code TS; `pytest`: `E   SyntaxError`/collection error). Suite fail CÓ named test mà không dính compile signature → assertion failure.
- `T-4` TurnResult nhận 2 signal caller-computed (cùng contract với `DodTransitionedToDone`/`TamperedTestPaths` — flowgate không tự chạy I/O): `ReproduceExpected bool` + `ReproduceCompileFailed bool`; danh sách test fail mới lấy từ `TurnResult.Tests.Failed` lọc theo test file vừa viết.
- `T-5` Fallback theo CP-64 §8: env `FLOWPILOT_ENABLE_REPRODUCE_GATE` unset/tắt → rule không được append, hành vi byte-identical hiện tại (signature rỗng).
- `T-6` R-1 (CP-64 §9) flaky-test mitigation: retry tối đa 2 lần (cấu hình được) thực hiện ở tầng runner/oracle suite-run (nơi gọi `executeSuite`), KHÔNG trong `flowgate.Evaluate` (flowgate pure, không I/O theo T-4) — chỉ khi kết quả fail không ổn định giữa 2 lần chạy liên tiếp (pass lúc reproduce nhưng fail lúc validate); classify lại pass/fail theo lần chạy cuối; retry không áp dụng cho compile error (deterministic).

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — `ReproduceRule`, `classifySuiteOutput`, gate wiring không nhận `providerKey` và không nhánh theo provider (verify bằng grep `providerKey|ProviderKey` = 0 hit trước khi đóng); 1 representative test đủ.
- Prior CA: none (new feature `reproduce-first-gate`); không undo work cũ; đóng task phải kèm CA-NNN ledger entry.
- Không làm gãy `task-harness`/`vibe-sprint`/`rag-harness`: khi flag tắt hoặc node không phải reproduce, gate output phải giống hệt trước thay đổi (regression safety).
- GitNexus impact analysis trước khi sửa symbol trong `gate_hook.go` (d=1 callers: `interactive_service.go:4762`, `interactive_service.go:7765`).
- `r-reproduce` phải tương thích precedence CP-62 (Task-337): không phá routing owner_debate/requirement của `ResolvePrecedence`.

### Open Questions

- `Q-1` (từ CP-64) — CHỐT: gắn timeout 15s cho lệnh chạy test reproduce ở tầng runner wiring (thuộc T-4 `gate_hook.go` suite-run path), không thuộc flowgate; E2E dùng cùng timeout để deterministic.

### Source Refs

- CP-64 §3.1 (vòng đời kiểm duyệt), §4 P-1, test signatures 1–4.
- `internal/flowgate/rules.go` (Rule struct, DefaultRules), `internal/flowgate/r_requirement.go` (selective-append pattern), `internal/flowgate/oracle.go` (OracleResult, executeSuite output parsing), `internal/flowgate/evaluate.go` (checkRule switch theo Trigger), `internal/runner/gate_hook.go` (runFlowGateAtEpoch, proposal-turn r-tests/r-reg suppression precedent).

## 1. Goal

Cổng `r-reproduce` hoạt động trong flowgate engine: turn reproduce không tạo được test ĐỎ vì assertion (all-pass hoặc compile error) đều bị chặn bằng reprompt với thông điệp cụ thể; turn tạo được ≥1 assertion failure mới thì pass. Flag tắt → hệ thống chạy y như cũ.

## 2. Parent Links

- coding plan: `CP-64-Reproduce-First-TDD-Gate.md` P-1
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

CP-64 đã phê duyệt học bài DeepSeek/SWE-bench Reproduce-First: bug fix không được có bằng chứng "tự biên tự diễn" nữa. P-1 là hạ tầng engine — mọi slice sau (P-2 prompt, P-3 flow, P-4 E2E) đều tiêu thụ rule này.

## 4. Exact Change

- `T-1` **`internal/flowgate/reproduce_rule.go`** (new): `ReproduceRuleID = "r-reproduce"`; `ReproduceRule() Rule` (Scope `step`, Trigger `reproduce_not_demonstrated`, RequiredOutput `reproducing_failing_test`, Action `reprompt`). Handler trong `checkRule`-style logic: khi `ReproduceExpected=true` — (a) `ReproduceCompileFailed=true` → violation "test bị lỗi cú pháp, sửa cho compile được"; (b) không có test fail mới nào từ file test vừa viết → violation "test chạy Xanh, chưa tái hiện được bug"; (c) có ≥1 assertion failure mới → nil (pass).
- `T-2` **`internal/flowgate/oracle.go`**: helper phân loại `classifySuiteOutput(testCmd, output string) (compileFailed bool)` — nhận dạng compile-error signature cho go/npm/pytest; không đổi behavior của `executeSuite`/`RunOracleContext` (additive only).
- `T-3` **`internal/flowgate/rules.go`**: thêm `ReproduceExpected bool` + `ReproduceCompileFailed bool` vào `TurnResult` (json tag omitempty, caller-computed như nhóm Dod*).
- `T-4` **`internal/runner/gate_hook.go`** (`runFlowGateAtEpoch`): khi flow node active mang behavior `agent.reproduce` (hoặc `change_type: behavior-change` trong bug flow) VÀ `FLOWPILOT_ENABLE_REPRODUCE_GATE` bật — suppress `r-tests`/`r-reg` cho turn này (proposal-turn exemption pattern), populate 2 signal reproduce (compile classification từ `oracle.Output`), append `ReproduceRule()` vào rule set trước `flowgate.Evaluate`.
- `T-5` **`internal/flowgate/reproduce_rule_test.go`** (new): 4 test signatures dưới đây + case flag-tắt (rule không append, evaluate output không đổi).

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/flowgate/reproduce_rule.go` (new)
  - `apps/local-runner/internal/flowgate/reproduce_rule_test.go` (new)
  - `apps/local-runner/internal/flowgate/rules.go` (modified — TurnResult signals only)
  - `apps/local-runner/internal/flowgate/oracle.go` (modified — additive classifier helper)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified — wiring + suppression)
- modules: `flowgate`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Turn reproduce với `Tests.Ran=true`, 0 test fail → violation `r-reproduce` reprompt "chưa tái hiện được bug".
- [x] AC-2: `OracleResult.Output` chứa compile-error signature (ví dụ `go test` với `undefined: Foo`) → violation reprompt "sửa cho compile được"; KHÔNG được tính là reproduce thành công.
- [x] AC-3: Suite fail với ≥1 named assertion failure từ file test mới viết (`--- FAIL: TestX` không kèm compile signature) → KHÔNG violation (pass gate).
- [x] AC-4: Turn thường (không phải node reproduce, không phải behavior-change) → `r-reproduce` không xuất hiện trong evaluation; `r-tests`/`r-reg` hoạt động như cũ.
- [x] AC-5: `FLOWPILOT_ENABLE_REPRODUCE_GATE` unset → rule không append, suppress không xảy ra, hành vi byte-identical với trước thay đổi.
- [x] AC-6: 4 test signatures green:
  - `TestRuleReproduceFailsWhenAllTestsPass`
  - `TestRuleReproduceFailsOnCompileError`
  - `TestRuleReproducePassesOnAssertionFailure`
  - `TestRuleReproduceSkipsOnNonBugFlow`
- [x] AC-7: Flaky reproduce run (fail lần 1, pass ở retry) → sau số lần retry cấu hình (mặc định 2) không sinh violation `r-reproduce` sai; compile error không bao giờ được retry.

## 7. Out of Scope

- Prompt template + agent persona + đăng ký behavior `agent.reproduce` (P-2 / Task-365).
- Sửa `bug-harness.yaml`/`bug-plan-harness.yaml` và khóa file test (P-3 / Task-366).
- E2E lifecycle (P-4 / Task-367).
- Thay đổi semantics của `r-tests`/`r-reg` ngoài phạm vi suppress đúng 1 turn reproduce.

## 8. Completion Notes

- result: done 2026-09-16 — `reproduce_rule.go` (ReproduceRuleID `r-reproduce`, Trigger `reproduce_not_demonstrated`), `classifySuiteOutput` go/npm/pytest, TurnResult signals, gate-hook selective append + 1-turn `r-tests`/`r-reg` suppression, 15s suite-run timeout at runner layer (Q-1). AC-1→AC-7 ticked; P-1 tests green.
- follow-ups: none — consumed by Task-365/366/367.
- upstream docs updated: CA-872 (change-audit/CA-872-CP-64-Reproduce-First-TDD-Gate-Closeout.md).
