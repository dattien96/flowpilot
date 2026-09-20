# CP-64: Reproduce-First TDD Gate (`r-reproduce`)

## Metadata

- Document ID: `CP-64`
- Title: `Reproduce-First TDD Gate`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SP-06: Oracle Rule And Schema First Gate](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)
- Child Documents: [Task-364](../../08-Task/done/Task-364-Reproduce-Gate-Rule-FlowGate.md) (P-1), [Task-365](../../08-Task/done/Task-365-Reproducer-Prompt-Agent-Behavior.md) (P-2), [Task-366](../../08-Task/done/Task-366-Bug-Flows-Reproduce-Node-Test-Lock.md) (P-3), [Task-367](../../08-Task/done/Task-367-Bug-Fix-Lifecycle-E2E-Reproduce-Gate.md) (P-4)
- Related Documents: [CP-55: Flow-First Preflight Contract](../done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [CP-62: Zcode Harness Parity](../done/CP-62-Zcode-Harness-Parity.md), [CP-63: IDE-Grade LSP Runtime](CP-63-IDE-Grade-LSP-Runtime.md)
- Replaces: `None`
- Tags: `tdd, reproduce-first, oracle-rule, flowgate, bugfix-safety, red-green`
- Feature Keys: `reproduce-first-gate`
- Implementation Owner: `Claude Sonnet MAX`

---

## AI Quick View

### Summary

- Quy trình TDD hiện tại của FlowPilot dùng empty test signatures (`func TestFoo(t *testing.T) {}`), dẫn đến việc test không bao giờ chạy ĐỎ lúc có bug, và Coder có thể tự điền assertion khớp với cách nó vừa sửa (confirmation bias).
- Học tập DeepSeek / SWE-bench Reproduce-First: Trước khi cho phép sửa code production trong BugFix, Agent Tester bắt buộc phải viết 1 test hoàn chỉnh và test đó PHẢI CHẠY ĐỎ (assertion failure, không phải compile error).
- CP-64 bổ sung cổng kiểm duyệt mới `r-reproduce` vào `flowgate`, tạo prompt template `prompts/reproduce-failing-test.md`, áp dụng có chọn lọc cho `bug-harness`, `bug-plan-harness` và các task có `change_type: behavior-change`.
- Các task tính năng mới hoàn toàn tiếp tục dùng `test_signatures` rỗng để tránh kẹt lỗi compiler.

### Current Ask

- Triển khai 4 slice công việc (P-1 đến P-4) bổ sung rule `r-reproduce`, phân biệt Assertion Failure vs Compile Error, cập nhật flow bugfix và bảo đảm tính tương thích ngược.

### Key Decisions

- `P-1` Cổng `r-reproduce` chỉ kích hoạt trên các node có behavior `agent.reproduce` hoặc node `test_signatures` trong các flow fix bug; không chặn các flow task tính năng mới.
- `P-2` Runner phân biệt rõ: nếu test fail do Compile Error (lỗi cú pháp) $\rightarrow$ KHÔNG đạt chuẩn reproduce (chặn). Chỉ khi compile thành công và fail do Assertion Error (`expected X got Y`) $\rightarrow$ mới ĐẠT chuẩn reproduce.
- `P-3` Sau khi test ĐỎ đã pass cổng `r-reproduce`, file test đó bị khóa quyền ghi (read-only) đối với node Coder, ngăn chặn Coder sửa lại test để né lỗi.

### Constraints

- Không làm gãy quy trình tạo tính năng mới của `task-harness` và `vibe-sprint`.
- Additive tests only — không sửa test cũ.
- GitNexus impact analysis trước mỗi symbol edit.

### Open Questions

- `Q-1` — CHỐT (Task-364): gắn timeout 15s cho lệnh chạy test reproduce ở tầng runner wiring, không thuộc flowgate.

### Source Refs

- SS-14; SD-20; SP-06 (Oracle Rule).
- DeepSeek SWE-bench methodology (Test-time compute reproduction phase).
- `apps/local-runner/internal/runner/gate_hook.go` (Oracle test evaluation).

---

## 1. Goal

Hiện thực hóa cổng kiểm duyệt TDD Đỏ-Xanh chuẩn mực (Reproduce-First Gate) cho toàn bộ quy trình sửa bug của FlowPilot. Đảm bảo 100% bug được fix đều có bằng chứng vật lý: test đã chạy ĐỎ trước khi sửa, và chuyển sang XANH sau khi sửa, mà không cho phép Coder "tự biên tự diễn" test assertion.

---

## 2. Input Documents

### 2.1 Governing documents
- SS-14: Code Context And Regression Safety.
- SD-20: Flow Gate Rule Semantics.
- SP-06: Oracle Rule And Schema First Gate.

### 2.2 Implemented foundations
- CP-55: Flow-First Preflight Contract & Scope Freeze.
- CP-62: Node Isolation & Gate Precedence.

### 2.3 Current code locations
- `apps/local-runner/internal/runner/gate_hook.go`: Nơi thực thi các gate rule (`r-tests`, `r-scope`, `r-contract`).
- `apps/local-runner/internal/flowgate/rules.go`: Bộ định nghĩa luật của gate.
- `apps/local-runner/internal/agentpack/flow-pack/flows/bug-harness.yaml`: Flow xử lý bug.
- `apps/local-runner/internal/agentpack/flow-pack/prompts/test-signatures.md`: Prompt TDD hiện tại.

---

## 3. Implementation Strategy

### 3.1 Vòng đời kiểm duyệt của `r-reproduce`
```text
Node: test_reproduce
  ├── Agent viết file test mới
  ├── Runner chạy validation command
  └── Gate r-reproduce đánh giá:
        ├── Compile Error? ──► REPROMPT: "Test bị lỗi cú pháp, sửa cho compile được"
        ├── All Tests Pass? ──► REPROMPT: "Test chạy Xanh, chưa tái hiện được bug"
        └── Test Failed by Assertion? ──► PASS GATE! Khóa file test, chuyển sang Coder.
```

### 3.2 Khóa quyền ghi file test ở lượt Coder
Khi chuyển sang node `implement`:
- File test vừa viết ở bước reproduce được thêm vào danh sách `read_only_paths` của node Coder.
- Coder chỉ được phép sửa file trong `DeclaredPaths` của production code.

---

## 4. Work Breakdown

### P-1: Rule `r-reproduce` trong FlowGate Engine

**Status: done** (Task-364, CA-872)

**Production changes**
- `internal/flowgate/reproduce_rule.go` (**new**): Định nghĩa rule `r-reproduce`. Kiểm tra `TurnResult.Tests`:
  - Yêu cầu: Có ít nhất 1 test mới fail.
  - Phải phân biệt rõ `Oracle.CompileError` (false) vs `Oracle.AssertionFailure` (true).
- `internal/flowgate/oracle.go` (modified, additive): helper `classifySuiteOutput(testCmd, output)` nhận dạng compile-error signature go/npm/pytest; không đổi `executeSuite` behavior.
- `internal/flowgate/rules.go` (modified, additive): thêm `ReproduceExpected` + `ReproduceCompileFailed` vào `TurnResult` (caller-computed, omitempty).
- `internal/runner/gate_hook.go`: Bổ sung kiểm tra `r-reproduce` vào `runFlowGateAtEpoch` (suppress `r-tests`/`r-reg` đúng 1 turn reproduce; timeout suite-run 15s ở tầng runner, không thuộc flowgate — Q-1 đã chốt).

**Test signatures**
```go
func TestRuleReproduceFailsWhenAllTestsPass(t *testing.T)
func TestRuleReproduceFailsOnCompileError(t *testing.T)
func TestRuleReproducePassesOnAssertionFailure(t *testing.T)
func TestRuleReproduceSkipsOnNonBugFlow(t *testing.T)
```

---

### P-2: Prompt Template & Behavior `agent.reproduce`

**Status: done** (Task-365, CA-872)

**Production changes**
- `internal/agentpack/flow-pack/prompts/reproduce-failing-test.md` (**new**): Prompt hướng dẫn viết test tái hiện có assertion đầy đủ, cấm chạm vào production code.
- `internal/agentpack/flow-pack/agents/reproducer.md` (**new**): Agent persona chuyên biệt cho việc tái hiện bug.
- `internal/agentpack/flow-pack/manifest.yaml` (modified): đăng ký 2 file mới.
- `internal/runner/behavior_registry_builtin.go` + `behaviors/registry.yaml` (modified): đăng ký behavior `agent.reproduce` (scope `delegate`, handler `behaviorAgentDelegate`, write-scope chỉ file test mới).

**Test signatures**
```go
func TestReproducerAgentPromptRender(t *testing.T)
func TestReproducerArtifactBinding(t *testing.T)
```

---

### P-3: Cập nhật các Flow Fix Bug (`bug-harness`, `bug-plan-harness`)

**Status: done** (Task-366, CA-872)

**Production changes**
- Cập nhật `bug-harness.yaml` và `bug-plan-harness.yaml`: Thay thế node `test_signatures` bằng node `reproduce_test` với behavior `agent.reproduce` và gate rule `r-reproduce`.
- Thêm cơ chế khóa file test vừa tạo thành read-only cho node `implement` tiếp theo (`changecontract.FrozenRecord.ReadOnlyPaths` + enforce tại `turnBridge.RequestApproval`; resolve legacy theo flag qua `resolveReproducePrompt`).
- `prompts/implement-complete-tests.md` (modified, additive 1 đoạn): file test reproduce đã khóa — chỉ chạy và giữ xanh.

**Test signatures**
```go
func TestBugHarnessTopologyContainsReproduceGate(t *testing.T)
func TestBugPlanHarnessTopologyContainsReproduceGate(t *testing.T)
func TestCoderNodeHasTestFileAsReadOnly(t *testing.T)
```

---

### P-4: E2E Validation & Safe-Fix Parity

**Status: done** (Task-367, CA-872)

**Production changes**
- Viết test E2E mô phỏng toàn bộ chu trình fix bug: từ test đỏ $\rightarrow$ coder fix $\rightarrow$ test xanh $\rightarrow$ gate pass.

**Test signatures**
```go
func TestBugFixLifecycleEndToEndWithReproduceGate(t *testing.T)
func TestBugFixFailsClosedWhenBugNotReproduced(t *testing.T)
```

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/flowgate/reproduce_rule.go` (new)
  - `apps/local-runner/internal/flowgate/reproduce_rule_test.go` (new)
  - `apps/local-runner/internal/flowgate/oracle.go` (modified — additive classifier)
  - `apps/local-runner/internal/flowgate/rules.go` (modified — TurnResult signals)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified)
  - `apps/local-runner/internal/changecontract/frozen_scope.go` (modified — ReadOnlyPaths)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/reproduce-failing-test.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/agents/reproducer.md` (new)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (modified — agent.reproduce)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/implement-complete-tests.md` (modified — additive)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/bug-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/bug-plan-harness.yaml` (modified)
- modules: `flowgate`, `runner`, `agentpack`

## 6. Data or Migration Steps

- schema: none
- data backfill: none

## 7. Validation Plan

- tests to add: ~11 tests
- failure cases: Model viết test bị lỗi cú pháp, model viết test pass ngay từ đầu.

## 8. Rollout and Fallback

- Rollout: P-1 $\rightarrow$ P-2 $\rightarrow$ P-3 $\rightarrow$ P-4.
- Fallback: Nếu tắt cờ `FLOWPILOT_ENABLE_REPRODUCE_GATE`, flow tự động fallback về cơ chế signature rỗng cũ.
- **Retired (CP-67, B-9):** cờ `FLOWPILOT_ENABLE_REPRODUCE_GATE` không còn tác dụng — reproduce-first gate luôn bật (`ReproduceGateEnabled()` luôn trả `true`). Rollback path là revert commit, không phải flag flip. Xem [CP-67](../todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md) / CA-896.

## 9. Risks

- `R-1` **Flaky Test:** Test chập chờn có thể pass lúc reproduce nhưng fail lúc validate. Mitigation: Cho phép cấu hình retry 2 lần.

## 10. Definition of Done

- [x] Rule `r-reproduce` chặn đứng các lượt test không fail do assertion.
- [x] Compile error không được tính là reproduce thành công.
- [x] Coder node bị khóa quyền sửa file test vừa được tạo.
- [x] `bug-harness` và `bug-plan-harness` chạy mượt mà end-to-end với quy trình mới.
- [x] Toàn bộ test additive đều green.

---

## Review Protocol

1. Implementer hoàn thành từng slice P-* và tạo Task + CA doc.
2. Reviewer kiểm tra điều kiện chuyển màu Đỏ $\rightarrow$ Xanh.
3. Không chấp nhận PR nếu thiếu bằng chứng test ĐỎ trước khi sửa code.
