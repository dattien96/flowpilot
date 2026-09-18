# CP-67: Contract-First Scaffold TDD & Signature Lock Gate

## Metadata

- Document ID: `CP-67`
- Title: `Contract-First Scaffold TDD & Signature Lock Gate`
- Phase: `coding_plan`
- Status: `ready`
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude Sonnet MAX, Operator`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SP-06: Oracle Rule And Schema First Gate](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)
- Child Documents: [Task-378](../../08-Task/todo/Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-379](../../08-Task/todo/Task-379-Signature-Lock-Rule-AST-Extractor.md), [Task-380](../../08-Task/todo/Task-380-Scaffold-Architect-Prompt-Agent-Persona.md), [Task-381](../../08-Task/todo/Task-381-Coder-Scaffold-Body-Prompt-Batch-Contract.md), [Task-382](../../08-Task/todo/Task-382-Flow-Topology-Scaffold-Renegotiation-Loop.md)
- Related Documents: [CP-64: Reproduce-First TDD Gate](../done/CP-64-Reproduce-First-TDD-Gate.md), [CP-62: Zcode Harness Parity](../done/CP-62-Zcode-Harness-Parity.md), [CP-63: IDE-Grade LSP Runtime](../done/CP-63-IDE-Grade-LSP-Runtime.md)
- Replaces: `None`
- Tags: `tdd, contract-first, scaffold, signature-lock, flowgate, schema-first, batch-renegotiate, task-harness, vibe-sprint`
- Feature Keys: `contract-first-tdd`

---

## AI Quick View

### Summary

- CP-64 đã giải quyết triệt để TDD cho BugFix bằng cơ chế Reproduce-First (test phải ĐỎ trước khi sửa code), nhưng đối với Task tính năng mới (`task-harness`, `vibe-sprint`), hệ thống vẫn dùng empty test signatures, đẩy rủi ro Confirmation Bias ("test tự khen mình") về bước Coder.
- CP-67 thiết lập giao thức **Contract-First Scaffold TDD**: TDD step sử dụng High-Reasoning Model ("model xịn") để sinh bộ khung Production Stubs (`TODO("not implemented")`, `return nil`) và bộ test suite hoàn chỉnh (Executable Tests).
- Test bắt buộc phải Compile thành công 100% nhưng chạy ĐỎ (RED) ở runtime. Khi pass, test files bị khóa `ReadOnlyPaths`, và toàn bộ chữ ký API được snapshot thành `SignatureHash`.
- Coder step chỉ được phép fill body code và tuyệt đối KHÔNG được sửa signature (`r-signature-lock`). Nếu phát hiện signature cần đổi, Coder không dừng lẻ tẻ mà tiếp tục code, gom thành **1 Batch Request duy nhất** ở cuối turn.
- Mọi giao tiếp đàm phán signature bắt buộc phải qua **Main Agent (Hub/Orchestrator)** làm trung gian thẩm định; tuyệt đối cấm peer-to-peer giữa Coder và TDD. Mọi kết quả trả về bắt buộc tuân thủ Typed Tool Schemas (`submit_scaffold_outcome`, `submit_coder_outcome`) với ngân sách an toàn `cap: 5`.

### Current Ask

- Xây dựng 5 slice công việc (P-1 đến P-5) hiện thực hóa: Schema tool faces, Gate `r-signature-lock` (AST Canonical Hash), TDD Scaffolding Prompts & Personas, Coder Batching Accumulation Contract, và Topology Flow renegotiation loop qua Main Agent với `cap: 5`.

### Key Decisions

- `P-1` Toàn bộ kết quả trả về của TDD và Coder bắt buộc dùng Declared Face Tool Schemas (`submit_scaffold_outcome`, `submit_coder_outcome`); cấm parse prose text tự do (tuân thủ SP-06 Tier 1).
- `P-2` Kiểm duyệt Signature Lock (`r-signature-lock`) bằng cơ chế băm chuẩn hóa AST: trích xuất các khai báo hàm/struct/interface (loại bỏ phần thân hàm `{ ... }`), chuẩn hóa alphabet và băm SHA256. Mọi sai khác signature ở Coder turn đều bị từ chối.
- `P-3` TDD node chuyển sang sử dụng High-Reasoning Model (Claude Sonnet/Opus, Grok 4.6 Reasoning, Codex) đóng vai trò Contract Architect: sinh file stub hợp lệ về cú pháp kèm test assertions hoàn chỉnh.
- `P-4` Coder thực thi theo nguyên tắc "Accumulate & Batch": khi phát hiện signature bất cập, Coder ghi nhận vào danh sách tạm, tiếp tục implement tối đa các phần khác của task, và chỉ nộp 1 Batch Request duy nhất ở cuối turn.
- `P-5` Giao thức đàm phán hợp đồng là Hub-and-Spoke: Coder nộp batch về Main Agent $\rightarrow$ Main Agent thẩm định $\rightarrow$ Main Agent giao TDD sửa $\rightarrow$ Main Agent bàn giao lại cho Coder. Ngân sách vòng lặp được cấu hình cứng `cap: 5`.

### Constraints

- Không làm gãy quy trình `bug-harness` đã ổn định của CP-64.
- Đảm bảo tương thích đa ngôn ngữ (Go, Kotlin/Android KMP, TypeScript).
- Không phá vỡ quy tắc Additive tests only (SP-06).
- Mọi điều hướng trạng thái phải dựa trên typed tool schemas, không dùng heuristic text matching.

### Open Questions

- `Q-1`: AST parser cho Go có sẵn (`go/parser`), nhưng với Kotlin và TypeScript thì trích xuất signature bằng công cụ gì? $\rightarrow$ Sử dụng LSP Document Symbols (`textDocument/documentSymbol`) từ CP-63 LSP Runtime hoặc Regex Normalizer có cấu trúc.

### Source Refs

- [SP-06: The Oracle Rule & Schema-First Gate Enforcement](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)
- [CP-64: Reproduce-First TDD Gate](../done/CP-64-Reproduce-First-TDD-Gate.md)
- [CP-63: IDE-Grade LSP Runtime](../done/CP-63-IDE-Grade-LSP-Runtime.md)
- `apps/local-runner/internal/runner/gate_hook.go`
- `apps/local-runner/internal/changecontract/frozen_scope.go`

---

## 1. Goal

Xóa bỏ hoàn toàn "lỗ hổng" Confirmation Bias (Coder tự viết test để tự pass code của mình) trong các luồng phát triển tính năng mới (`task-harness`, `vibe-sprint`). Thiết lập cơ chế kiểm soát chất lượng 2 tầng: Model cao cấp thiết kế khung API và bộ test ĐỎ có khả năng thực thi, Coder chỉ điền logic và bị khóa chữ ký, cùng cơ chế đàm phán chữ ký theo lô (Batching) qua Main Agent điều phối.

---

## 2. Input Documents

### 2.1 Governing documents
- [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- [SP-06: The Oracle Rule & Schema-First Gate Enforcement](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)

### 2.2 Implemented foundations
- [CP-55: Flow-First Preflight Contract & Scope Freeze](../done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- [CP-62: Node Isolation & Gate Precedence](../done/CP-62-Zcode-Harness-Parity.md)
- [CP-64: Reproduce-First TDD Gate](../done/CP-64-Reproduce-First-TDD-Gate.md)

---

## 3. Implementation Strategy

### 3.1 Luồng Vòng Đời Contract-First TDD
```text
  ┌─────────────────────────────────────────────────────────────┐
  │ 1. TDD Step (High-Reasoning Model)                          │
  │    - Đọc Task Plan / Specs                                  │
  │    - Viết Production Stubs (TODO / return null)             │
  │    - Viết Full Executable Test Suite (Chạy ĐỎ)              │
  │    - Gọi Tool: submit_scaffold_outcome                      │
  └──────────────────────────────┬──────────────────────────────┘
                                 │
                                 ▼
  ┌─────────────────────────────────────────────────────────────┐
  │ 2. Runner Gate Check & Lock                                 │
  │    - Compile Check: PASS 100% (Không có compile error)      │
  │    - Runtime Test Check: RED 100% (NotImplemented/Assertion)│
  │    - Runner khóa Test File thành ReadOnlyPaths              │
  │    - Runner snapshot SHA256 SignatureHash                   │
  └──────────────────────────────┬──────────────────────────────┘
                                 │
                                 ▼
  ┌─────────────────────────────────────────────────────────────┐
  │ 3. Coder Step (Implementer)                                 │
  │    - Điền thân hàm (Fill body only)                         │
  │    - Cấm sửa test (Bị chặn ở bridge)                        │
  │    - Cấm sửa signature (Gate r-signature-lock)              │
  │    - Nếu phát hiện signature bất cập:                       │
  │      -> Tạm nhớ lại, code tiếp hết mức có thể               │
  │      -> Gom toàn bộ thành 1 BATCH REQUEST cuối turn         │
  │    - Gọi Tool: submit_coder_outcome                         │
  └──────────────────────────────┬──────────────────────────────┘
                                 │
         ┌───────────────────────┴───────────────────────┐
         │ (status: completed)                           │ (status: renegotiate_signatures)
         ▼                                               ▼
  ┌─────────────────────────────┐         ┌─────────────────────────────┐
  │ 4A. Validate & Review       │         │ 4B. Main Agent Hub Mediation│
  │    - Test XANH 100%         │         │    - Thẩm định Batch Request│
  │    - Forward to Reviewer    │         │    - Duyệt/Lọc các signature│
  └─────────────────────────────┘         │    - Loop về TDD (cap: 5)   │
                                          └──────────────┬──────────────┘
                                                         │
                                                         ▼
                                          ┌─────────────────────────────┐
                                          │ TDD cập nhật Stub & Test    │
                                          │ rồi giao lại Coder turn sau │
                                          └─────────────────────────────┘
```

### 3.2 Cơ Chế Signature Lock Hash (AST Normalization)
1. **Trích xuất Signature**:
   - Chỉ lấy các khai báo Symbol (Hàm, Method, Interface, Struct, Class).
   - Loại bỏ hoàn toàn khối thân hàm `{ ... }`.
2. **Canonical Hash**:
   - Sắp xếp danh sách symbols theo thứ tự chuẩn hóa alphabet.
   - Băm SHA256 chuỗi canonical $\rightarrow$ `SignatureHash`.
3. **Đánh giá tại Coder Turn**:
   - Trước Coder: `Hash_TDD`.
   - Sau Coder: `Hash_Coder`.
   - Nếu `Hash_Coder != Hash_TDD` và Coder không nộp kèm batch renegotiation $\rightarrow$ Vi phạm `r-signature-lock`, reprompt bắt hoàn tác thay đổi signature.

---

## 4. Work Breakdown

### P-1: Declared Face Tool Schemas (`flow-pack/tools`)

**Status: todo**

**Production changes**
- `internal/agentpack/flow-pack/tools/submit-scaffold-outcome.yaml` (**new**): Tool face cho TDD step:
  - Input: `status` (`scaffold_ready`, `blocked`), `stubs` (file, symbols), `test_suite` (test_file, red_tests, failure_type).
- `internal/agentpack/flow-pack/tools/submit-coder-outcome.yaml` (**new**): Tool face cho Coder step:
  - Input: `status` (`completed`, `renegotiate_signatures`, `blocked`), `summary`, `batch_signature_requests` (array: symbol, file, current_signature, proposed_signature, rationale), `implementation_progress`.
- `internal/agentpack/flow-pack/manifest.yaml` (modified): Đăng ký 2 tool faces mới.

**Test signatures**
```go
func TestSubmitScaffoldOutcomeSchemaValidation(t *testing.T)
func TestSubmitCoderOutcomeBatchSignatureValidation(t *testing.T)
```

---

### P-2: Rule `r-signature-lock` & AST Signature Extractor

**Status: todo**

**Production changes**
- `internal/flowgate/signature_lock_rule.go` (**new**): Định nghĩa rule `r-signature-lock`. So sánh `TurnResult.SignatureHash` với baseline `FrozenContractRecord.SignatureHash`.
- `internal/flowgate/ast_signatures.go` (**new**): Bộ trích xuất chữ ký chuẩn hóa (hỗ trợ Go qua `go/parser`, Kotlin/TS qua LSP document symbols / regex extractor).
- `internal/changecontract/preflight.go` (modified): Thêm `SignatureHash string` và `LockedSignatures []string` vào `FrozenContractRecord`.
- `internal/runner/gate_hook.go` (modified): Đánh giá `r-signature-lock` tại lượt Coder.

**Test signatures**
```go
func TestExtractCanonicalSignaturesGo(t *testing.T)
func TestExtractCanonicalSignaturesKotlin(t *testing.T)
func TestRuleSignatureLockFailsOnSignatureModification(t *testing.T)
func TestRuleSignatureLockPassesWhenOnlyBodyModified(t *testing.T)
```

---

### P-3: Scaffold TDD Prompts & Agent Persona (High-Reasoning Tier)

**Status: todo**

**Production changes**
- `internal/agentpack/flow-pack/agents/scaffold-architect.md` (**new**): Persona cho TDD Agent định vị vai trò là API Contract Architect.
- `internal/agentpack/flow-pack/prompts/scaffold-contract-tdd.md` (**new**): Prompt hướng dẫn chi tiết:
  1. Tạo file stub với hàm rỗng (`TODO()`, `return nil`).
  2. Viết toàn bộ test assertions hoàn chỉnh gọi stub.
  3. Chạy test xác nhận Compile XANH và Test ĐỎ.
  4. Gọi tool `submit_scaffold_outcome`.
- `internal/runner/context_profiles.go` (modified): Cấu hình model tier cao (high reasoning) cho scaffold node.

**Test signatures**
```go
func TestScaffoldArchitectPromptRendering(t *testing.T)
func TestScaffoldOutcomeExecutionStatus(t *testing.T)
```

---

### P-4: Coder Prompts & In-Turn Accumulation Contract

**Status: todo**

**Production changes**
- `internal/agentpack/flow-pack/prompts/implement-scaffold-body.md` (**new**): Prompt thay thế `implement-complete-tests.md` cho các flow áp dụng:
  1. Chỉ điền thân hàm.
  2. Tuyệt đối không sửa test (đã khóa read-only).
  3. Tuyệt đối không sửa signature trực tiếp.
  4. Khi phát hiện signature cần đổi: nhớ lại, code tiếp tối đa, cuối turn gom vào `batch_signature_requests` của `submit_coder_outcome`.

**Test signatures**
```go
func TestImplementScaffoldBodyPrompt(t *testing.T)
func TestCoderAccumulatedBatchPayloadHandling(t *testing.T)
```

---

### P-5: Flow Topology Updates & Central Main Agent Loop (`task-harness`, `vibe-sprint`)

**Status: todo**

**Production changes**
- `internal/agentpack/flow-pack/flows/task-harness.yaml` (modified):
  - Cập nhật node `test_signatures` thành node `scaffold_tdd` với behavior `agent.delegate`, agent `agents/scaffold-architect.md`.
  - Cập nhật node `implement` dùng prompt `implement-scaffold-body.md`.
  - Bổ sung routing qua Main Agent (hub):
    `implement` $\rightarrow$ `main_hub` (when: renegotiate) $\rightarrow$ `scaffold_tdd` (when: continue, kind: back).
  - Khóa cứng `cap: 5` cho phase negotiation.
- `internal/agentpack/flow-pack/flows/vibe-sprint.yaml` (modified): Tương thích cấu trúc mới cho `tdd` và `coder`.

**Test signatures**
```go
func TestTaskHarnessTopologyScaffoldNegotiationLoop(t *testing.T)
func TestMainAgentMediationDispatchesBatchToTDD(t *testing.T)
func TestRenegotiationCapEnforcedAtFive(t *testing.T)
```

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/tools/submit-scaffold-outcome.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/tools/submit-coder-outcome.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/agents/scaffold-architect.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/scaffold-contract-tdd.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/implement-scaffold-body.md` (new)
  - `apps/local-runner/internal/flowgate/signature_lock_rule.go` (new)
  - `apps/local-runner/internal/flowgate/ast_signatures.go` (new)
  - `apps/local-runner/internal/changecontract/preflight.go` (modified)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml` (modified)
- modules: `flowgate`, `changecontract`, `agentpack`, `runner`

---

## 6. Data or Migration Steps

- schema: Bổ sung field `signature_hash` và `locked_signatures` vào schema `frozen_contracts.ndjson`.
- data backfill: Không cần backfill (các contract cũ không có signature_hash sẽ coi như bỏ qua check này).

---

## 7. Validation Plan

- tests to add: ~15 unit và E2E tests.
- failure cases:
  - Coder tự ý đổi kiểu tham số $\rightarrow$ Gate `r-signature-lock` chặn.
  - TDD viết test không compile được $\rightarrow$ Bị chặn tại TDD gate.
  - TDD viết test chạy xanh ngay từ đầu $\rightarrow$ Bị chặn tại TDD gate.
  - Coder gửi lẻ tẻ thay vì gom batch $\rightarrow$ Main Agent reprompt yêu cầu hoàn thành turn và gom batch.

---

## 8. Rollout and Fallback

- Rollout: P-1 $\rightarrow$ P-2 $\rightarrow$ P-3 $\rightarrow$ P-4 $\rightarrow$ P-5.
- Fallback: Cờ môi trường `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD=false` sẽ fallback về hành vi empty signature cũ.

---

## 9. Risks

- `R-1` **Lệch AST đa ngôn ngữ**: Việc bóc tách signature giữa Go, Kotlin, TypeScript có cú pháp khác nhau.
  *Mitigation*: Sử dụng LSP symbol extraction từ CP-63 làm chuẩn chung; fallback sang AST chuẩn của từng ngôn ngữ khi LSP không khả dụng.

---

## 10. Definition of Done

- [ ] Cổng `r-signature-lock` chặn đứng mọi nỗ lực sửa chữ ký của Coder.
- [ ] TDD sinh ra Stubs và Test ĐỎ có thể compile được 100%.
- [ ] Coder gom toàn bộ yêu cầu đổi signature thành 1 Batch gửi về Main Agent.
- [ ] Main Agent làm trung gian điều phối và loop về TDD với `cap: 5`.
- [ ] 100% kết quả chuyển giao sử dụng Typed Tool Schemas.
