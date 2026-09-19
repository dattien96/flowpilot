# CP-67: Contract-First Scaffold TDD & Signature Lock Gate

## Metadata

- Document ID: `CP-67`
- Title: `Contract-First Scaffold TDD & Signature Lock Gate`
- Phase: `coding_plan`
- Status: `ready`
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude Sonnet MAX, Operator`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
- Parent Documents: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SP-06: Oracle Rule And Schema First Gate](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)
- Child Documents: [Task-378](../../08-Task/todo/Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-379](../../08-Task/todo/Task-379-Signature-Lock-Rule-AST-Extractor.md), [Task-380](../../08-Task/todo/Task-380-Scaffold-Architect-Prompt-Agent-Persona.md), [Task-381](../../08-Task/todo/Task-381-Coder-Scaffold-Body-Prompt-Batch-Contract.md), [Task-382](../../08-Task/todo/Task-382-Flow-Topology-Scaffold-Renegotiation-Loop.md), [Task-383](../../08-Task/todo/Task-383-Multi-Language-Stub-Body-Validation.md)
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
- Post-review B-11: tín hiệu detect thứ 3 cho `r-scaffold-red` — **static stub-body whitelist**: AST walk kiểm tra thân mọi symbol chỉ chứa dạng stub chuẩn (TODO/throw not-implemented/return zero-sentinel), deterministic và không phụ thuộc kết quả test (đóng lỗ hổng "viết body nhưng logic sai nên test vẫn ĐỎ"). Hỗ trợ 4 nhóm ngôn ngữ: Go (`go/parser`), React TS/TSX (`node` + TypeScript Compiler API), Kotlin (LSP-anchored), C/C++ (tree-sitter sau build tag `treesitter`, mặc định clangd LSP).

### Current Ask

- Xây dựng 5 slice công việc (P-1 đến P-5) hiện thực hóa: Schema tool faces, Gate `r-signature-lock` (AST Canonical Hash), TDD Scaffolding Prompts & Personas, Coder Batching Accumulation Contract, và Topology Flow renegotiation loop qua Main Agent với `cap: 5`.

### Key Decisions

- `P-1` Toàn bộ kết quả trả về của TDD và Coder bắt buộc dùng Declared Face Tool Schemas (`submit_scaffold_outcome`, `submit_coder_outcome`); cấm parse prose text tự do (tuân thủ SP-06 Tier 1).
- `P-2` Kiểm duyệt Signature Lock (`r-signature-lock`) bằng cơ chế băm chuẩn hóa AST: trích xuất các khai báo hàm/struct/interface (loại bỏ phần thân hàm `{ ... }`), chuẩn hóa alphabet và băm SHA256. Mọi sai khác signature ở Coder turn đều bị từ chối.
- `P-3` TDD node chuyển sang sử dụng High-Reasoning Model (Claude Sonnet/Opus, Grok 4.6 Reasoning, Codex) đóng vai trò Contract Architect: sinh file stub hợp lệ về cú pháp kèm test assertions hoàn chỉnh.
- `P-4` Coder thực thi theo nguyên tắc "Accumulate & Batch": khi phát hiện signature bất cập, Coder ghi nhận vào danh sách tạm, tiếp tục implement tối đa các phần khác của task, và chỉ nộp 1 Batch Request duy nhất ở cuối turn.
- `P-5` Giao thức đàm phán hợp đồng là Hub-and-Spoke: Coder nộp batch về Main Agent $\rightarrow$ Main Agent thẩm định $\rightarrow$ Main Agent giao TDD sửa $\rightarrow$ Main Agent bàn giao lại cho Coder. Ngân sách vòng lặp được cấu hình cứng `cap: 5`.
- `P-2b` (B-11 / Task-383) Detect body logic ở scaffold turn bằng **static stub-body whitelist** (tín hiệu thứ 3, ngoài GREEN-test và compile-fail): một lần AST walk trả về cả signature lẫn `BodyShape` của mỗi symbol; body nào ngoài whitelist shape (TODO/throw not-implemented/return zero-sentinel) → violation. Fail-open khi parser không khả dụng (`body_unverified` + evidence). Rollout theo ngôn ngữ: Go → React → Kotlin → C/C++; land sau P-5, không block flow sống.

### Constraints

- Không làm gãy quy trình `bug-harness` đã ổn định của CP-64.
- Đảm bảo tương thích đa ngôn ngữ (Go, Kotlin/Android KMP, TypeScript/React, C/C++).
- Giả định harness đã biết cách build/run test theo ngôn ngữ của repo đích (`go test` / gradle / jest / ctest-cmake) — CP-67 không xây test-runner mới.
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
  │    - Runtime Test Check: RED (≥1 test ĐỎ)                    │
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

> **B-11 bổ sung cho bước 2 (Runner Gate Check & Lock)**: sau khi test ĐỎ, gate chạy thêm **static stub-body whitelist** — thân mọi symbol phải nằm trong whitelist dạng stub (chi tiết §3.2 tín hiệu 3); vi phạm → reprompt viết lại stub rỗng trước khi khóa test + snapshot hash.

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
4. **Detect AI viết body logic trong TDD step** — 3 tín hiệu (B-11 bổ sung tín hiệu 3):
   - Tín hiệu 1 (GREEN): `r-scaffold-red` phát hiện `Tests.Failed == 0` (test chạy XANH) → AI đã viết body logic thực tế (không phải stub rỗng) → reprompt yêu cầu AI viết lại thành stub.
   - Tín hiệu 2 (COMPILE): `r-scaffold-red` phát hiện `ScaffoldCompileFailed` → AI viết code không compile được → reprompt.
   - Tín hiệu 3 (STATIC — B-11/Task-383): **stub-body whitelist** — AST walk kiểm tra thân mọi symbol chỉ chứa dạng stub chuẩn (Go: `return nil, errors.New("not implemented")`/`panic(...)`; Kotlin: `= TODO(...)`/`throw NotImplementedError(...)`; TS/React: `throw new Error("not implemented")`/`=> TODO()`; C: `return` zero-sentinel/`assert(0)`; C++: `throw std::runtime_error(...)`/`return nullptr`). Body có `if/for/switch`, nhiều statement, hoặc call ngoài whitelist → violation `NonStub` → reprompt. Tín hiệu này **deterministic, không phụ thuộc test ĐỎ hay XANH** — đóng lỗ hổng "viết body nhưng logic sai nên test vẫn ĐỎ".
   - Chính sách fail-open: parser không khả dụng (LSP off, tree-sitter không build, dialect lạ) → symbol đánh dấu `body_unverified`, check bỏ qua nhưng ghi evidence; parse thành công mà body non-stub → luôn violation.

**Kết luận**: TDD step detect được AI viết body thực tế bằng 3 tín hiệu: (1) test chạy GREEN (body logic đã implement), (2) code không compile, (3) body ngoài stub whitelist (static). Nếu 1 trong 3 xảy ra → rule force AI phải viết lại thành **stub rỗng (signature only)**. Rule `r-scaffold-red` yêu cầu tối thiểu 1 test ĐỎ (`len(Tests.Failed) > 0`) — không yêu cầu RED 100% vì (a) test cấu trúc (reflection/compile-only) có thể hợp lệ xanh, (b) tín hiệu static (B-11) đã chặn smuggling body bất kể kết quả test.

---

## 4. Work Breakdown

### P-1: Declared Face Tool Schemas & Provider Exposure (`flow-pack/tools` + 4 adapters)

**Status: todo** — mở rộng post-review B-1/B-2: không chỉ 2 YAML; phải wire face tới cả 4 provider và expose cho child delegate node.

**Production changes**
- `internal/agentpack/flow-pack/tools/submit-scaffold-outcome.yaml` (**new**): TDD face — `status` (`scaffold_ready`→done, `blocked`→escalate), `stubs` (file, symbols), `test_suite` (test_file, red_tests, failure_type).
- `internal/agentpack/flow-pack/tools/submit-coder-outcome.yaml` (**new**): Coder face — `status` (`completed`→done, `renegotiate_signatures`→continue, `blocked`→escalate), `summary`, `batch_signature_requests` (symbol, file, current_signature, proposed_signature, rationale), `implementation_progress`.
- `internal/agentpack/flow-pack/manifest.yaml` (modified): đăng ký 2 faces.
- **Provider exposure (B-2)** — mỗi adapter phải khai schema + allow-flag + dispatch:
  - `internal/runner/claude_mcp_server.go`: 2 tool def + 2 case trong `tools/call` + 2 allow-map.
  - `internal/runner/claude_permission_mcp.go`: `handleClaudeSubmitScaffoldOutcome` / `handleClaudeSubmitCoderOutcome` → `bridge.SubmitFlowControl`.
  - `internal/runner/codex_adapter.go`: `flowpilot_submit_scaffold_outcome` / `flowpilot_submit_coder_outcome`, dynamic tool def, allow-flag, dispatch.
  - `internal/runner/grok_adapter.go`, `internal/runner/opencode_adapter.go`: MCP register + allow-flag (parity).
- **TurnRequest flags (B-1)**: `internal/runner/provider_registry.go` thêm `OfferScaffoldOutcomeTool` / `OfferCoderOutcomeTool`; `internal/runner/interactive_service.go` set theo behavior node đang chạy. Child call là **record-only**, payload buffer (`pendingBatchSignatureByStep` — pattern `pendingReviewVerdictByLabel`, `review_done_verdict.go:81-96`) rồi hub đọc khi mediate.
- `internal/runner/agent_orchestrator.go`: `resolveFaceStatus` hỗ trợ 2 face mới qua `agentpack.LoadBuiltinToolFace`.

**Test signatures**
```go
func TestSubmitScaffoldOutcomeSchemaValidation(t *testing.T)
func TestSubmitCoderOutcomeBatchSignatureValidation(t *testing.T)
func TestScaffoldOutcomeToolAdvertisedOnScaffoldNode(t *testing.T)
func TestCoderOutcomeToolAdvertisedOnImplementNode(t *testing.T)
func TestCoderOutcomeChildCallIsRecordOnlyAndBuffered(t *testing.T)
```

---

### P-2: Rules `r-signature-lock` + `r-scaffold-red` & AST Signature Extractor

**Status: todo** — mở rộng post-review B-4/B-8: thêm `r-scaffold-red`, signature hash (chỉ signature, không bao gồm body), LSP extractor, single-write lock+snapshot.

**Production changes**
- `internal/flowgate/signature_lock_rule.go` (**new**): rule `r-signature-lock` — so `TurnResult.SignatureHashBefore` vs `SignatureHashAfter`; bypass khi `CoderRenegotiating=true`. Pattern `reproduce_rule.go` (không trong `DefaultRules()`).
- `internal/flowgate/scaffold_red_rule.go` (**new**, B-4): rule `r-scaffold-red` — scaffold turn phải `Tests.Ran && !ScaffoldCompileFailed && len(Tests.Failed) > 0`; suppress `r-tests/r-reg` cho turn đó; ghi evidence red tests từ `test_suite.red_tests`. Tín hiệu 3 (static body whitelist — B-11) do Task-383 (P-2b) wire thêm qua `TurnResult.ScaffoldBodyNonStub`/`NonStubSymbols`.
- `internal/flowgate/ast_signatures.go` (**new**): extractor — Go dùng `go/parser`+`go/ast`; Kotlin/TS dùng **LSP `textDocument/documentSymbol`** (B-8.4), fallback regex khi LSP off. `CanonicalSignatureHash` sort+join+SHA256. **Signature-only strict**: mọi add/remove/modify symbol signature đều đổi hash — body được loại bỏ (B-8.1). Extractor trả về `SymbolInfo{Name, Kind, Signature, Line}`; Task-383 (P-2b) bổ sung `BodyShape` vào cùng struct — một lần walk, hai đầu ra.
- `internal/runner/lsp_client.go` (modified): thêm method `DocumentSymbols(filePath)`.
- `internal/flowgate/rules.go` (modified): `TurnResult` thêm `SignatureHashBefore/After`, `CoderRenegotiating`, `ScaffoldExpected`, `ScaffoldCompileFailed`.
- `internal/changecontract/preflight.go` (modified): `FrozenContractRecord` thêm `SignatureHash`, `LockedSignatures`.
- `internal/changecontract/frozen_scope.go` (modified, B-8.3): hàm mới `LockScaffoldArtifacts(store, existing, testPaths, signatureHash, lockedSignatures, now)` — ghi gộp **1 version bump** vào record step `implement`/`coder`.
- `internal/runner/gate_hook.go` (modified): scaffold turn arm `r-scaffold-red` + suppress test rules + on-pass gọi `LockScaffoldArtifacts`; coder turn populate hash before/after + `CoderRenegotiating` + append `r-signature-lock`.
- `internal/runner/reproduce_gate.go` (modified, B-9): retire `FLOWPILOT_ENABLE_REPRODUCE_GATE` (`ReproduceGateEnabled()` luôn true); generalize lock predicate `IsReproduceBehavior || IsScaffoldBehavior`; xóa legacy degrade (`resolveReproducePrompt`/`resolveReproduceAgent`).

**Test signatures**
```go
func TestExtractCanonicalSignaturesGo(t *testing.T)
func TestCanonicalSignatureHashDeterministic(t *testing.T)
func TestExtractCanonicalSignaturesKotlinViaLSP(t *testing.T)
func TestRuleSignatureLockFailsOnSignatureModification(t *testing.T)
func TestRuleSignatureLockFailsOnAdditiveFunction(t *testing.T)
func TestRuleSignatureLockPassesWhenOnlyBodyModified(t *testing.T)
func TestRuleSignatureLockBypassesWhenRenegotiating(t *testing.T)
func TestRuleScaffoldRedRequiresFailingTests(t *testing.T)
func TestRuleScaffoldRedRejectsCompileFailure(t *testing.T)
func TestRuleScaffoldRedRejectsAllGreen(t *testing.T)
func TestRuleScaffoldRedSuppressesTestRules(t *testing.T)
func TestRuleScaffoldRedDetectsBodyLogicAsGreenTests(t *testing.T)
func TestRuleScaffoldRedForcesStubAfterBodyViolation(t *testing.T)
func TestLockScaffoldArtifactsSingleVersionBump(t *testing.T)
func TestReproduceGateRetiredAlwaysOn(t *testing.T)
```

---

### P-2b: Multi-Language Stub-Body Validation & Language Adapters (`B-11` / Task-383)

**Status: todo** — tín hiệu detect thứ 3 cho `r-scaffold-red`: static stub-body whitelist qua AST, 4 nhóm ngôn ngữ. Land **sau P-5**, không block flow sống (gate chạy với 2 tín hiệu ban đầu).

**Production changes**
- `internal/flowgate/ast_signatures.go` (modified): `SymbolInfo` thêm `BodyShape` (`StubTodo | StubThrow | StubReturnZero | NonStub | Unverified`) + `Line int`; `CanonicalSignatureHash` giữ nguyên signature-only (B-8.1 compat).
- `internal/flowgate/stub_bodies.go` (**new**): `ValidateStubBodies(lang, symbols) []BodyViolation` — whitelist shape per ngôn ngữ (xem §3.2 tín hiệu 3); xử lý nơi logic trốn: Go package `var` initializer + `init()` rỗng; Kotlin `init {}`/property initializer/default param/companion; TS class field arrow + top-level const; C/C++ macro đa-statement (flag NonStub) + global initializer có call.
- `internal/flowgate/lang_adapter_react.go` (**new**): subprocess `node` + `internal/flowgate/scripts/extract-ts.mjs` (TypeScript Compiler API, `jsx: true`) → JSON `SymbolInfo`; `.jsx` không type: signature = name + param list text + arity.
- `internal/flowgate/lang_adapter_kotlin.go` (**new**): LSP-anchored — `documentSymbol` range + `foldingRange` → cắt body text theo tọa độ → shape match (dùng `lsp_client.go` từ CP-63).
- `internal/flowgate/lang_adapter_cpp.go` (**new**, build tag `treesitter`): tree-sitter-c/cpp exact parser; build `!treesitter`: clangd LSP anchored fallback. C/C++ decl-vs-def dedupe theo canonical signature (`.h` + `.c` = 1 symbol); thêm `#include` không tính là symbol change.
- `internal/flowgate/stub_body_cache.go` (**new**): cache kết quả parse theo `(path, mtime, content hash)` — gate chạy mỗi turn nhưng chỉ re-parse file đổi.
- `internal/flowgate/rules.go` (modified): `TurnResult` thêm `ScaffoldBodyNonStub bool` + `NonStubSymbols []string` (json omitempty).
- `internal/flowgate/scaffold_red_rule.go` (modified): thêm nhánh (d) `ScaffoldBodyNonStub` → violation reprompt chỉ đích danh symbol + line.
- `internal/runner/gate_hook.go` (modified): scaffold turn gọi adapters + populate 2 field mới.

**Test signatures** (15)
```go
func TestValidateStubBodiesGo(t *testing.T)
func TestValidateStubBodiesGoRejectsNonStubLogic(t *testing.T)
func TestExtractCanonicalSignaturesReactViaNode(t *testing.T)
func TestValidateStubBodiesReactViaNode(t *testing.T)
func TestValidateStubBodiesReactRejectsNonStubLogic(t *testing.T)
func TestValidateStubBodiesKotlinViaLSP(t *testing.T)
func TestExtractCanonicalSignaturesCppViaTreeSitter(t *testing.T)
func TestValidateStubBodiesCppViaTreeSitter(t *testing.T)
func TestCppMacroBodyHidingDetected(t *testing.T)
func TestCppDeclDefDedupeSignatureHashStable(t *testing.T)
func TestSignatureHashUnchangedWithBodyShapeOutput(t *testing.T)
func TestStubBodyCacheInvalidation(t *testing.T)
func TestStubBodyUnverifiedFailOpen(t *testing.T)
func TestRuleScaffoldRedFiresOnNonStubBody(t *testing.T)
func TestStubBodyAdaptersDispatchByLanguage(t *testing.T)
```

---

### P-3: Scaffold TDD Prompts, Agent Persona & Behavior `agent.scaffold` (High-Reasoning Tier)

**Status: todo** — sửa post-review B-3/B-5: bỏ `context_profiles.go` (file không tồn tại); model default đặt ở P-5 (flow YAML `model:`); behavior `agent.scaffold` đăng ký ở cả agentpack lẫn runner.

**Production changes**
- `internal/agentpack/flow-pack/agents/scaffold-architect.md` (**new**): Persona API Contract Architect.
- `internal/agentpack/flow-pack/prompts/scaffold-contract-tdd.md` (**new**): Prompt — (1) tạo stub `TODO()/return nil/throw Error`; (2) viết test assertions hoàn chỉnh; (3) chạy test xác nhận Compile XANH + Test ĐỎ; (4) gọi `submit_scaffold_outcome`.
- `internal/agentpack/flow-pack/manifest.yaml` (modified).
- **Behavior registration (B-3)**:
  - `internal/agentpack/pack.go`: alias `"agent.scaffold"` (+ `scaffold`, `scaffold_tdd`) vào `behaviorAliases`; mở model-allowlist (line ~1020-1035) cho `agent.scaffold` và `agent.code` (B-5).
  - `internal/agentpack/flow_safety_topology.go`: classify `agent.scaffold` như writer (dominated-by-freeze + acceptance).
  - `internal/agentpack/flow-pack/behaviors/registry.yaml` (modified — reference doc).
  - `internal/runner/behavior_registry.go`: `BehaviorAgentScaffold`, `IsScaffoldBehavior(id)`; scaffold node là frozen-contract writer (stub trong DeclaredPaths), file test mới miễn scope-drift (IsTestFile exemption, pattern reproduce).
  - `internal/runner/behavior_registry_builtin.go`: register handler (reuse `behaviorAgentDelegate`).
  - `internal/runner/flow_validate_audit_dispatch.go`: thêm `agent.scaffold` vào spawnable-target allowlist (~line 225) và `flowAgentCodeWriterNodes` (~line 2183).
  - `internal/runner/flow_executor.go` + `internal/runner/artifact_type_registry.go`: agent-name/prompt resolution cho behavior mới (không cần degrade vì không flag).

**Test signatures**
```go
func TestScaffoldArchitectPromptRendering(t *testing.T)
func TestScaffoldBehaviorRegistration(t *testing.T)
func TestScaffoldNodeAllowsModelField(t *testing.T)
```

---

### P-4: Coder Prompts & In-Turn Accumulation Contract

**Status: todo**

**Production changes**
- `internal/agentpack/flow-pack/prompts/implement-scaffold-body.md` (**new**): Prompt thay thế `implement-complete-tests.md` cho các flow áp dụng:
  1. Chỉ điền thân hàm.
  2. Tuyệt đối không sửa test (đã khóa read-only).
  3. Tuyệt đối không sửa signature, **không thêm/xóa function** (kể cả helper — B-8.1).
  4. Khi phát hiện signature cần đổi: nhớ lại, code tiếp tối đa, cuối turn gom vào `batch_signature_requests` của `submit_coder_outcome`.
- `internal/agentpack/flow-pack/manifest.yaml` (modified).

**Test signatures**
```go
func TestImplementScaffoldBodyPromptRendering(t *testing.T)
func TestImplementScaffoldBodyPromptContainsBatchContract(t *testing.T)
```

---

### P-5: Flow Topology Updates & Central Main Agent Loop (`task-harness`, `vibe-sprint`)

**Status: todo** — **sửa post-review B-6/B-7/B-9/B-10**: giữ nguyên node id (`test_signatures`, `tdd`), thêm node `synthesis_negotiation`, phase-scoped `negotiationCap: 5`, không flag runtime, không wire per-face constant vào adapter — harness uses existing `submit_review_outcome` tool + `SubmitFlowControl` bridge.

**Production changes**
- `internal/agentpack/flow-pack/flows/task-harness.yaml` (modified):
  - Node `test_signatures` (**id giữ nguyên** — B-7): behavior `agent.code` → `agent.scaffold`; agent `agents/test/test.md` → `agents/scaffold-architect.md`; promptTemplate `prompts/test-signatures.md` → `prompts/scaffold-contract-tdd.md`; lifecycle `reinvoke`; thêm `model: claude-sonnet-4-5` (high-reasoning default; admin override qua `step_definitions` — B-5, `pack.go:1020-1035` mở cho `agent.scaffold`).
  - Node `implement`: promptTemplate `prompts/implement-complete-tests.md` → `prompts/implement-scaffold-body.md`.
  - tools list: thêm `tools/submit-scaffold-outcome.yaml`, `tools/submit-coder-outcome.yaml` (harness-declared logical faces; B-1 decision — adapter-level exposure dùng tool cũ).
  - Node trung gian `synthesis_negotiation` (inline hub, `hub.inline`, `agents/synthesizer.md`): nhận continuación hóa renegotiation từ coder step, thẩm định batch, forward cho `test_signatures` → `implement`. Không constraint mới; reuse `SubmitFlowControl` bridge.
  - Edges (B-6): `synthesis → synthesis_negotiation` (when: `continue` với payload batch renegotiation), `synthesis_negotiation → test_signatures` (when: `continue`, kind: `back`), `synthesis_negotiation → synthesis` (when: `done`, kind: `forward`). Caps negotiation phase: `policy.negotiationCap: 5` (B-10).
  - Node mới `synthesis_negotiation` là inline hub `hub.inline` → không cần spawnable-target change; reuse `flowValidateStatusMap`.
- `internal/agentpack/flow-pack/flows/vibe-sprint.yaml` (modified):
  - Node `tdd` (**id giữ nguyên** — B-7): behavior `agent.code` → `agent.scaffold`; agent `agents/test/test.md` → `agents/scaffold-architect.md`; promptTemplate `prompts/test-signatures.md` → `prompts/scaffold-contract-tdd.md`; lifecycle `reinvoke`; `model: claude-sonnet-4-5`; contextProfile `tdd` giữ nguyên.
  - Node `coder`: promptTemplate `prompts/implement-complete-tests.md` → `prompts/implement-scaffold-body.md`.
  - tools list: thêm `tools/submit-scaffold-outcome.yaml`, `tools/submit-coder-outcome.yaml`.
  - Node `synthesis_negotiation` (inline hub, `hub.inline`, `agents/synthesizer.md`).
  - Edges (B-6): `coder → synthesis_negotiation` (when: `continue` với renegotiation batch), `synthesis_negotiation → tdd` (when: `continue`, kind: `back`), `synthesis_negotiation → synthesis` (when: `done`, kind: `forward`). `policy.negotiationCap: 5`.
  - Không đổi edge `synthesis → coder (when:continue, kind:back)` (cũ) — vẫn dùng cho review loop; renegotiation route qua `synthesis_negotiation`.
- `internal/agentpack/pack.go` (modified): parse `policy.negotiationCap` (default 5); validate range 1..20.
- `internal/runner/agent_orchestrator.go` (modified): `AgentLoopState` thêm `NegotiationRound int`; `effectiveNegotiationCap` = `policy.negotiationCap` (trong flow policy) else 5; mỗi back-edge negotiation: increment `NegotiationRound`; nếu `>= negotiationCap` → escalate (không extend); reset khi `synthesis_negotiation → synthesis (done)` (phase code hoàn thành).
- `internal/runner/interactive_service.go` (modified): routing `synthesis_negotiation → test_signatures/tdd (continue)` dispatch qua `SubmitFlowControl` bridge (B-1 resolve).
- `internal/runner/gate_hook.go` (modified): scaffold turn pass `r-scaffold-red` → gọi `LockScaffoldArtifacts` (B-8.3); coder turn → populate hash before/after + `CoderRenegotiating`; roadmap: `submit_review_outcome` status `renegotiate_signatures` (canonical term — Task-378 T-5) → `continue` trong statusMap bridge.
- **Không có `scaffold_fallback.go`, không env flag** (B-9); CP-67 + CP-64 (reproduce) always-on; legacy prompt (`test-signatures.md`, `test-signatures.md`, `implement-complete-tests.md`) giữ lại — chỉ dùng cho rollback revert commit / bug-harness legacy.

**Test signatures**
```go
func TestTaskHarnessTopologyScaffoldNegotiationLoop(t *testing.T)
func TestVibeSprintTopologyScaffoldNegotiationLoop(t *testing.T)
func TestScaffoldNegotiationCapEnforcedAtFive(t *testing.T)
func TestSynthesisNegotiationNodeRouting(t *testing.T)
func TestScaffoldTestLockAndSignatureSnapshotSingleVersion(t *testing.T)
func TestLegacyTestSignaturesFlowUnaffected(t *testing.T)
```

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/tools/submit-scaffold-outcome.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/tools/submit-coder-outcome.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/agents/scaffold-architect.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/scaffold-contract-tdd.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/implement-scaffold-body.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (modified — đăng ký 2 faces + agent + prompt)
  - `apps/local-runner/internal/agentpack/pack.go` (modified — behaviorAliases + model-allowlist + policy.negotiationCap)
  - `apps/local-runner/internal/agentpack/flow_safety_topology.go` (modified — classify agent.scaffold như writer)
  - `apps/local-runner/internal/agentpack/flow-pack/behaviors/registry.yaml` (modified — reference doc)
  - `apps/local-runner/internal/flowgate/signature_lock_rule.go` (new)
  - `apps/local-runner/internal/flowgate/scaffold_red_rule.go` (new)
  - `apps/local-runner/internal/flowgate/ast_signatures.go` (new)
  - `apps/local-runner/internal/flowgate/stub_bodies.go` (new — B-11)
  - `apps/local-runner/internal/flowgate/lang_adapter_react.go` (new — B-11)
  - `apps/local-runner/internal/flowgate/scripts/extract-ts.mjs` (new — B-11)
  - `apps/local-runner/internal/flowgate/lang_adapter_kotlin.go` (new — B-11)
  - `apps/local-runner/internal/flowgate/lang_adapter_cpp.go` (new — B-11, build tag `treesitter`)
  - `apps/local-runner/internal/flowgate/stub_body_cache.go` (new — B-11)
  - `apps/local-runner/internal/flowgate/rules.go` (modified — TurnResult 5 fields mới: SignatureHashBefore, SignatureHashAfter, CoderRenegotiating, ScaffoldExpected, ScaffoldCompileFailed; Task-383 bổ sung ScaffoldBodyNonStub, NonStubSymbols)
  - `apps/local-runner/internal/runner/lsp_client.go` (modified — DocumentSymbols method)
  - `apps/local-runner/internal/changecontract/preflight.go` (modified — 2 fields mới: SignatureHash, LockedSignatures)
  - `apps/local-runner/internal/changecontract/frozen_scope.go` (modified — LockScaffoldArtifacts)
  - `apps/local-runner/internal/runner/behavior_registry.go` (modified — BehaviorAgentScaffold, IsScaffoldBehavior)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (modified — register handler)
  - `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (modified — spawnable Target + flowAgentCodeWriterNodes)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified — scaffold-red arm + lock + hash snapshot)
  - `apps/local-runner/internal/runner/reproduce_gate.go` (modified — retire flag, legacy degrade xóa)
  - `apps/local-runner/internal/runner/provider_registry.go` (modified — không thêm flag mới, dùng existing submit_review_outcome)
  - `apps/local-runner/internal/runner/agent_orchestrator.go` (modified — NegotiationRound, negotiationCap)
  - `apps/local-runner/internal/runner/interactive_service.go` (modified — routing synthesis_negotiation)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml` (modified)
  - `requirements/05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md` (modified — AC-21)
  - `requirements/05-System-Specs/SS-18-Vibe-Working-Mode.md` (modified — TDD bullet)
  - `requirements/05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md` (modified — AC-9, siết AC-2)
  - `requirements/06-System-Tech-Design/SD-24-Vibe-Working-Mode.md` (modified — tdd node semantics, synthesis_negotiation topology)
  - `requirements/04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md` (modified — precedence)
  - `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md` (modified — §2.10 r-signature-lock, §2.11 r-scaffold-red, §7 precedence)
  - `requirements/07-Coding-Plan/todo/CP-67-Test-Steps.md` (new)
  - `change-audit/CA-XXX-*.md` (new per slice)
  - `requirements/07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md` (modified — flag retire note)
- modules: `flowgate`, `changecontract`, `agentpack`, `runner`
- routes: `POST /client/workflow-runs/{run}/flow-control` (existing)

## 6. Data or Migration Steps

- schema: Bổ sung `signature_hash` (string) + `locked_signatures` (array of strings) vào `FrozenContractRecord`; thêm `NegotiationRound int` + `NegotiationCap int` vào `AgentLoopState`.
- data backfill: Không cần backfill (các contract cũ không có `signature_hash` sẽ coi như bỏ qua check này — `omitempty` compat).
- **node id unchanged**: `test_signatures`, `tdd`, `implement`, `coder` giữ nguyên → không cần migrate `step_definitions` rows, không cần re-seed Supabase flow definition mirror.
- adapter-level: không thêm per-face constant vào 4 adapter — dùng existing `submit_review_outcome` tool (sau này nếu cần expose `submit_coder_outcome` riêng thì thêm slice sau).

---

## 7. Validation Plan

- tests to add: **46 test signatures** — P-1 (5), P-2 (15), P-2b/Task-383 (15), P-3 (3), P-4 (2), P-5 (6).
- failure cases (enforceable by gate/test):
  - Coder tự ý đổi signature cũ (tham số/return type) → Gate `r-signature-lock` chặn (signature hash lệch, không batch).
  - Coder tự thêm function helper mới → Gate `r-signature-lock` chặn (hash lệch, không batch) — strict semantics (B-8.1).
  - Coder tự xóa function → Gate `r-signature-lock` chặn.
  - TDD viết test không compile được → Bị chặn tại TDD gate (`r-scaffold-red`, `ScaffoldCompileFailed`).
  - TDD viết test chạy xanh ngay từ đầu → Bị chặn tại TDD gate (`r-scaffold-red`, không RED).
  - TDD viết body logic trong stub nhưng test vẫn ĐỎ (logic sai/nửa vời) → static stub-body whitelist (B-11/Task-383) chặn — reprompt viết lại stub rỗng.
  - Coder gửi lẻ tẻ thay vì gom batch → Main Agent (synthesis_negotiation) reprompt yêu cầu hoàn thành turn và gom batch.
  - Negotiation loop vượt `negotiationCap: 5` → escalate (không extend).
  - `bug-harness`, `rag-harness`, `bug-plan-harness` không bị ảnh hưởng (node id giữ nguyên, legacy prompt vẫn dùng).
- verification commands:
  - `go test ./internal/flowgate/... -run 'TestRuleSignatureLock|TestRuleScaffoldRed|TestCanonicalSignatureHash'` (P-2)
  - `go test ./internal/flowgate/... -run 'TestValidateStubBodies|TestStubBody|TestCpp|TestExtractCanonicalSignaturesReact|TestSignatureHashUnchanged'` (P-2b)
  - `go test ./internal/agentpack/... -run 'TestScaffoldBehaviorRegistration|TestScaffoldNodeAllowsModelField'` (P-3)
  - `go test ./internal/agentpack/flow-pack/flows/... -run 'TestTaskHarnessTopology|TestVibeSprintTopology|TestScaffoldNegotiationCap|TestSynthesisNegotiationRouting'` (P-5)
  - `go test ./internal/runner/... -run 'TestLegacyTestSignaturesFlowUnaffected'` (impact check)

---

## 8. Rollout and Fallback

- Rollout: P-1 → P-2 → P-3 → P-4 → P-5 → P-2b (Task-383, B-11 — land sau P-5, không block flow sống) → P-6. P-6 là slice doc-sync closeout theo convention (không có task doc riêng — deliverable là `CP-67-Test-Steps.md` live evidence + CA notes).
- **Không có env flag** để bật/tắt (B-9): CP-67 luôn bật khi merge; CP-64 `FLOWPILOT_ENABLE_REPRODUCE_GATE` cũng retire (always-on). Fallback = revert commit.
- Legacy path (`FLOWPILOT_ENABLE_REPRODUCE_GATE=false` behavior) không còn tồn tại như runtime option; legacy prompt files (`test-signatures.md`, `implement-complete-tests.md`) giữ lại như artifact (rollback revert, hoặc cho bug-harness legacy nếu cần).

---

## 9. Risks

- `R-1` **Lệch AST đa ngôn ngữ**: Việc bóc tách signature giữa Go, Kotlin, TypeScript có cú pháp khác nhau. *Mitigation*: Go dùng `go/parser`; Kotlin/TS ưu tiên LSP `documentSymbol` từ CP-63 (thêm method `DocumentSymbols` vào `lsp_client.go`); fallback regex khi LSP không khả dụng (post-review B-8.4).
- `R-2` **Provider face exposure scope** (post-review B-1/B-2 resolve): không wire per-face constant vào 4 adapter, dùng existing `submit_review_outcome` tool + `SubmitFlowControl` bridge. Rủi ro: batch_signature_requests typed data không truyền trực tiếp qua tool schema mới. *Mitigation*: coder báo renegotiation qua `submit_review_outcome` status `continue` + structured `issues[]` payload + harness context `synthesis_negotiation` đọc; TDD nhận batch từ harness context. Nếu sau này cần expose `submit_coder_outcome` tool riêng → thêm slice sau.
- `R-3` **Retire CP-64 flag impact**: `FLOWPILOT_ENABLE_REPRODUCE_GATE=false` behavior mất đi (always-on). *Mitigation*: test suite regress toàn bộ bug-harness với flag effectively always-on; legacy prompt giữ lại.
- `R-4` **Negotiation routing complexity**: 2 back-edges `continue` từ `synthesis_negotiation` (một tới `test_signatures/tdd`, một tới `synthesis` done). *Mitigation*: đường back-edge tới `test_signatures/tdd` khi có renegotiation batch; đường forward `done` tới `synthesis` khi negotiation xong; resolver keyed từ `when` + payload batch có mặt.
- `R-5` **Token cost của high-reasoning TDD model**: TDD dùng model mạnh hơn (`claude-sonnet-4-5`) mỗi turn. *Mitigation*: cap `negotiationCap: 5` giới hạn vòng lặp; thường 1-2 vòng là đủ.
- `R-6` **Multi-language AST accuracy & build tag cgo** (B-11): tree-sitter cần cgo; node/clangd/kotlin-LSP phụ thuộc toolchain repo đích. *Mitigation*: build mặc định `CGO_ENABLED=0` chạy adapters node/LSP (không cgo); build tag `treesitter` opt-in cho exact parser offline; fail-open `body_unverified` khi parser không khả dụng; rollout tăng dần Go → React → Kotlin → C/C++.

---

## 10. Definition of Done

- [ ] P-1: 2 tool faces `submit_scaffold_outcome` + `submit_coder_outcome` tạo + đăng ký trong manifest; pack validation pass; 5 test signatures xanh (schema validation + tool exposure + buffered submission).
- [ ] P-1: harness `task-harness.yaml` + `vibe-sprint.yaml` list 2 tools mới; harness-declared faces render được trong prompt (không wire per-face constant vào 4 adapter — dùng existing `submit_review_outcome` + `SubmitFlowControl` bridge, B-1/B-2 resolve).
- [ ] P-2: `r-signature-lock` + `r-scaffold-red` rules tạo; `ast_signatures.go` + `DocumentSymbols` vào LSP client; `FrozenContractRecord` + `TurnResult` + `AgentLoopState` mở rộng field; `LockScaffoldArtifacts` single-write; 15 test signatures xanh.
- [ ] P-2: `r-scaffold-red` enforce compile OK + RED test trở chuẩn; suppress `r-tests/r-reg` cho scaffold turn; evidence red tests ghi từ `test_suite.red_tests`.
- [ ] P-2b: static stub-body whitelist hoạt động cho 4 nhóm ngôn ngữ (Go, React TS/TSX, Kotlin, C/C++); `SignatureHash` không đổi khi thêm `BodyShape` (B-8.1 compat); caching theo mtime + fail-open `body_unverified`; 15 test signatures xanh; default build không cgo vẫn pass toàn bộ test (tree-sitter tests skip có đánh dấu khi tag off).
- [ ] P-3: `scaffold-architect.md` persona + `scaffold-contract-tdd.md` prompt tạo; `agent.scaffold` behavior đăng ký ở agentpack (alias, safety topology) + runner (registry, spawnable target, writer classification); model-allowlist mở cho `agent.scaffold`; 3 test signatures xanh.
- [ ] P-4: `implement-scaffold-body.md` prompt tạo + kiểm tra 3 lệnh cấm + batch contract; 2 test signatures xanh.
- [ ] P-5: `task-harness.yaml` + `vibe-sprint.yaml` topology đúng: giữ node id (`test_signatures`/`tdd`/`implement`/`coder`), thêm `synthesis_negotiation` node, edges negotiation đúng, `policy.negotiationCap: 5`; `NegotiationRound`/`NegotiationCap` trong loop state; 6 test signatures xanh; `bug-harness`/`rag-harness`/`bug-plan-harness` không đổi.
- [ ] P-5: sau scaffold turn pass gate → test files khóa `ReadOnlyPaths` + `SignatureHash` + `LockedSignatures` snapshot vào `FrozenContractRecord` (single write `LockScaffoldArtifacts`).
- [ ] P-5: không env flag → CP-67 always-on; CP-64 `FLOWPILOT_ENABLE_REPRODUCE_GATE` retire (always-on); legacy prompt giữ lại.
- [ ] P-6: `CP-67-Test-Steps.md` tạo với toàn bộ test case + verification commands + live evidence; CA cho mỗi slice; sync upstream SS-14/SS-18/SS-19/SP-06/SD-20/SD-24.
- [ ] DoD gating: 100% kết quả chuyển giao sử dụng Typed Tool Schemas (`submit_scaffold_outcome`, `submit_coder_outcome` YAML faces); harness uses existing `submit_review_outcome` + `SubmitFlowControl` bridge cho chạy thực.
