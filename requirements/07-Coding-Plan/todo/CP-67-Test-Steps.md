# CP-67 Test Steps & Live Evidence

## Metadata

- Document ID: `CP-67-Test-Steps`
- Title: `CP-67 Contract-First Scaffold TDD: Test Steps & Live Evidence`
- Phase: `coding_plan`
- Status: `todo`
- Parent Documents: [CP-67: Contract-First Scaffold TDD & Signature Lock Gate](../todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Related: [CP-64: Reproduce-First TDD Gate](../done/CP-64-Reproduce-First-TDD-Gate.md), [Task-378](../todo/Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-379](../todo/Task-379-Signature-Lock-Rule-AST-Extractor.md), [Task-380](../todo/Task-380-Scaffold-Architect-Prompt-Agent-Persona.md), [Task-381](../todo/Task-381-Coder-Scaffold-Body-Prompt-Batch-Contract.md), [Task-382](../todo/Task-382-Flow-Topology-Scaffold-Renegotiation-Loop.md), [Task-383](../todo/Task-383-Multi-Language-Stub-Body-Validation.md)
- Created: `2026-09-19`

---

## 1. P-1 Test Steps (Task-378: Scaffold & Coder Declared Face Tool Schemas)

### 1.1 Test 1: TestSubmitScaffoldOutcomeSchemaValidation

**Mục tiêu:** Xác nhận tool face `submit_scaffold_outcome` có schema đúng contract.

**Steps:**
1. Khởi tạo `SubmitScaffoldOutcomeInput` với:
   - `status: scaffold_ready`
   - `stubs: [{file: "service.go", symbols: [{name: "GetUser", kind: "function", signature: "func GetUser(id string) (*User, error)"}]}]`
   - `test_suite: {test_file: "service_test.go", red_tests: ["TestGetUser_ReturnsNotFoundError"], failure_type: "not_implemented"}`
2. Validate input qua `ValidateScaffoldOutcome`.
3. Xác nhận:
   - `status` enum có 2 giá trị: `scaffold_ready`, `blocked`.
   - `stubs[].symbols[].kind` enum có 5 giá trị: `function`, `method`, `interface`, `struct`, `class`.
   - `test_suite.failure_type` enum có 2 giá trị: `not_implemented`, `assertion_failure`.
   - `stubs` và `test_suite` là required khi `status=scaffold_ready`.

**Expected:** Validate pass, không error.

---

### 1.2 Test 2: TestSubmitCoderOutcomeBatchSignatureValidation

**Mục tiêu:** Xác nhận tool face `submit_coder_outcome` có schema đúng contract, kể cả batch_signature_requests.

**Steps:**
1. Khởi tạo `SubmitCoderOutcomeInput` với:
   - `status: renegotiate_signatures`
   - `summary: "Cần thêm tham số forceRefresh cho GetUser"`
   - `batch_signature_requests: [{symbol: "GetUser", file: "service.go", current_signature: "func GetUser(id string) (*User, error)", proposed_signature: "func GetUser(id string, forceRefresh bool) (*User, error)", rationale: "Cần bypass cache khi refresh"}]`
   - `implementation_progress: "85%"`
2. Validate input.
3. Xác nhận:
   - `status` enum có 3 giá trị: `completed`, `renegotiate_signatures`, `blocked`.
   - `batch_signature_requests[].rationale` là required.
   - `batch_signature_requests[].symbol`, `file`, `current_signature`, `proposed_signature` đều required.

**Expected:** Validate pass.

---

### 1.3 Test 3: TestScaffoldOutcomeToolAdvertisedOnScaffoldNode

**Mục tiêu:** Xác nhận tool face `submit_scaffold_outcome` được advertise trong prompt khi node là scaffold.

**Steps:**
1. Tạo mock harness với node `test_signatures` (behavior `agent.scaffold`).
2. Render prompt cho node.
3. Xác nhận prompt contains:
   - Hướng dẫn gọi `submit_scaffold_outcome`.
   - Schema `submit_scaffold_outcome` (status, stubs, test_suite).

**Expected:** Prompt chứa tool definition.

---

### 1.4 Test 4: TestCoderOutcomeToolAdvertisedOnImplementNode

**Mục tiêu:** Xác nhận tool face `submit_coder_outcome` được advertise trong prompt khi node là implement/coder.

**Steps:**
1. Tạo mock harness với node `implement` (behavior `agent.code`).
2. Render prompt cho node.
3. Xác nhận prompt contains:
   - Hướng dẫn gọi `submit_coder_outcome`.
   - Schema `submit_coder_outcome` (status, batch_signature_requests).

**Expected:** Prompt chứa tool definition.

---

### 1.5 Test 5: TestCoderOutcomeChildCallIsRecordOnlyAndBuffered

**Mục tiêu:** Xác nhận gọi `submit_coder_outcome` từ child delegate node là record-only (không advance flow trực tiếp) và payload được buffer để hub đọc.

**Steps:**
1. Simulate child delegate node call `submit_coder_outcome` với `status: renegotiate_signatures`.
2. Xác nhận:
   - Flow không advance trực tiếp (record-only).
   - Payload `batch_signature_requests` được buffer trong `pendingBatchSignatureByStep`.
   - Hub (synthesis_negotiation) có thể đọc buffer khi mediate.

**Expected:** Record-only, buffer populated.

---

## 2. P-2 Test Steps (Task-379: Signature Lock Rule & AST Signature Extractor)

### 2.1 Test 1: TestExtractCanonicalSignaturesGo

**Mục tiêu:** Xác nhận `ExtractCanonicalSignatures` cho Go file trả về deterministic sorted list.

**Steps:**
1. Tạo file Go test fixture:
   ```go
   package service

   type User struct { ID string }

   func GetUser(id string) (*User, error) { return nil, errors.New("not implemented") }
   func CreateUser(name string) (*User, error) { return nil, errors.New("not implemented") }
   type UserService interface { GetUser(id string) (*User, error) }
   ```
2. Gọi `ExtractCanonicalSignatures("service.go", "go")`.
3. Xác nhận kết quả là sorted list:
   - `func CreateUser(name string) (*User, error)`
   - `func GetUser(id string) (*User, error)`
   - `type UserService interface { GetUser(id string) (*User, error) }`
   - `type User struct { ID string }`

**Expected:** List sorted alphabetical, chứa tất cả declarations.

---

### 2.2 Test 2: TestCanonicalSignatureHashDeterministic

**Mục tiêu:** Xác nhận `CanonicalSignatureHash` trả về cùng SHA256 bất kể thứ tự khai báo.

**Steps:**
1. Tạo 2 tập signatures:
   - Set A: ["func B()", "func A()"]
   - Set B: ["func A()", "func B()"]
2. Tính `h1 = CanonicalSignatureHash(Set A)`, `h2 = CanonicalSignatureHash(Set B)`.
3. Xác nhận `h1 == h2`.

**Expected:** h1 == h2.

---

### 2.3 Test 3: TestExtractCanonicalSignaturesKotlinViaLSP

**Mục tiêu:** Xác nhận `ExtractCanonicalSignatures` cho Kotlin file dùng LSP `DocumentSymbols`.

**Steps:**
1. Enable LSP mock cho Kotlin.
2. Tạo file Kotlin test fixture:
   ```kotlin
   data class User(val id: String)
   fun getUser(id: String): User = TODO("not implemented")
   interface UserService { fun getUser(id: String): User }
   ```
3. Gọi `ExtractCanonicalSignatures("UserService.kt", "kotlin")`.
4. Xác nhận kết quả chứa:
   - `data class User(val id: String)`
   - `fun getUser(id: String): User`
   - `interface UserService { fun getUser(id: String): User }`

**Expected:** List chứa tất cả declarations, sorted.

---

### 2.4 Test 4: TestRuleSignatureLockFailsOnSignatureModification

**Mục tiêu:** Xác nhận `r-signature-lock` violation khi Coder sửa signature cũ.

**Steps:**
1. Setup: `FrozenContractRecord` có `SignatureHash = hash(["func GetUser(id string) (*User, error)"])`.
2. Simulate Coder turn với `SignatureHashAfter = hash(["func GetUser(id string, forceRefresh bool) (*User, error)"])`.
3. Set `CoderRenegotiating = false`.
4. Evaluate rule `r-signature-lock`.
5. Xác nhận: violation fired, reprompt detail chứa diff signatures.

**Expected:** Violation.

---

### 2.5 Test 5: TestRuleSignatureLockFailsOnAdditiveFunction

**Mục tiêu:** Xác nhận `r-signature-lock` violation khi Coder thêm function mới (signature hash (chỉ signature, không bao gồm body) - B-8.1).

**Steps:**
1. Setup: `FrozenContractRecord` có `SignatureHash = hash(["func GetUser(id string) (*User, error)"])`.
2. Simulate Coder turn với `SignatureHashAfter = hash(["func GetUser(id string) (*User, error)", "func validateID(id string) error"])`.
3. Set `CoderRenegotiating = false`.
4. Evaluate rule `r-signature-lock`.
5. Xác nhận: violation fired (hash lệch do thêm function).

**Expected:** Violation. **Không bypass** - không có "additive change hợp lệ".

---

### 2.6 Test 6: TestRuleSignatureLockPassesWhenOnlyBodyModified

**Mục tiêu:** Xác nhận `r-signature-lock` pass khi Coder chỉ sửa body.

**Steps:**
1. Setup: `FrozenContractRecord` có `SignatureHash = hash(["func GetUser(id string) (*User, error)"])`.
2. Simulate Coder turn với `SignatureHashAfter = hash(["func GetUser(id string) (*User, error)"])` (chỉ sửa body, signature không đổi).
3. Set `CoderRenegotiating = false`.
4. Evaluate rule `r-signature-lock`.
5. Xác nhận: không violation.

**Expected:** Không violation.

---

### 2.7 Test 7: TestRuleSignatureLockBypassesWhenRenegotiating

**Mục tiêu:** Xác nhận `r-signature-lock` bypass khi Coder nộp batch renegotiation.

**Steps:**
1. Setup: `SignatureHashBefore = hash(["func GetUser(id string) (*User, error)"])`.
2. Simulate Coder turn với `SignatureHashAfter = hash(["func GetUser(id string, forceRefresh bool) (*User, error)"])`.
3. Set `CoderRenegotiating = true`.
4. Evaluate rule `r-signature-lock`.
5. Xác nhận: không violation (bypass cho renegotiation flow).

**Expected:** Không violation.

---

### 2.8 Test 8: TestRuleScaffoldRedRequiresFailingTests

**Mục tiêu:** Xác nhận `r-scaffold-red` fire khi test không RED.

**Steps:**
1. Setup: scaffold turn với `Tests.Ran = true`, `ScaffoldCompileFailed = false`, `Tests.Failed = []` (all green).
2. Evaluate rule `r-scaffold-red`.
3. Xác nhận: violation fired (test không RED).

**Expected:** Violation.

---

### 2.9 Test 9: TestRuleScaffoldRedRejectsCompileFailure

**Mục tiêu:** Xác nhận `r-scaffold-red` fire khi compile fail.

**Steps:**
1. Setup: scaffold turn với `Tests.Ran = true`, `ScaffoldCompileFailed = true`, `Tests.Failed = []`.
2. Evaluate rule `r-scaffold-red`.
3. Xác nhận: violation fired (compile fail).

**Expected:** Violation.

---

### 2.10 Test 10: TestRuleScaffoldRedRejectsAllGreen

**Mục tiêu:** Xác nhận `r-scaffold-red` fire khi test all green (không có RED test).

**Steps:**
1. Setup: scaffold turn với `Tests.Ran = true`, `ScaffoldCompileFailed = false`, `Tests.Failed = []` (all green, không RED test).
2. Evaluate rule `r-scaffold-red`.
3. Xác nhận: violation fired.

**Expected:** Violation.

---

### 2.11 Test 11: TestRuleScaffoldRedSuppressesTestRules

**Mục tiêu:** Xác nhận `r-tests`/`r-reg` được suppress cho scaffold turn.

**Steps:**
1. Setup: scaffold turn với test RED (`Tests.Ran = true`, `ScaffoldCompileFailed = false`, `len(Tests.Failed) > 0`).
2. Suppress `r-tests`/`r-reg`.
3. Evaluate rules.
4. Xác nhận: `r-tests`/`r-reg` không fire, chỉ `r-scaffold-red` evaluate.

**Expected:** r-tests/r-reg suppressed.

---

### 2.12 Test 12: TestLockScaffoldArtifactsSingleVersionBump

**Mục tiêu:** Xác nhận `LockScaffoldArtifacts` ghi gộp 1 version bump.

**Steps:**
1. Setup: `FrozenContractRecord` existing với `Version = 1`.
2. Gọi `LockScaffoldArtifacts(store, existing, testPaths=["service_test.go"], signatureHash="abc123", lockedSignatures=["func GetUser(...)"], now)`.
3. Xác nhận:
   - Record mới có `Version = 2` (1 version bump, không 2).
   - `ReadOnlyPaths` chứa `service_test.go`.
   - `SignatureHash = "abc123"`.
   - `LockedSignatures` chứa `func GetUser(...)`.

**Expected:** Record mới với 1 version bump, chứa tất cả fields.

---

### 2.13 Test 13: TestRuleScaffoldRedDetectsBodyLogicAsGreenTests

**Mục tiêu:** Xác nhận `r-scaffold-red` detect AI viết body logic qua tín hiệu all-green (Tín hiệu 1).

**Steps:**
1. Setup: scaffold turn với `Tests.Ran = true`, `ScaffoldCompileFailed = false`, `Tests.Failed = []` (all-green).
2. Evaluate rule `r-scaffold-red`.
3. Xác nhận: violation fired với reprompt detail ghi rõ nguyên nhân all-green → AI đã viết body logic thực tế (stub chuẩn phải ĐỎ).

**Expected:** Violation, reprompt yêu cầu viết lại stub rỗng.

---

### 2.14 Test 14: TestRuleScaffoldRedForcesStubAfterBodyViolation

**Mục tiêu:** Xác nhận vòng reprompt trả scaffold về stub chuẩn (signature only).

**Steps:**
1. Turn 1: scaffold vi phạm (all-green) → `r-scaffold-red` fire.
2. Simulate reprompt turn: AI viết lại stub rỗng.
3. Turn 2: scaffold turn với `Tests.Ran = true`, `ScaffoldCompileFailed = false`, `len(Tests.Failed) > 0` (ĐỎ chuẩn not-implemented).
4. Xác nhận: turn 2 pass gate `r-scaffold-red`.

**Expected:** Sau reprompt, stub rỗng → test ĐỎ → gate pass.

---

### 2.15 Test 15: TestReproduceGateRetiredAlwaysOn

**Mục tiêu:** Xác nhận flag B-9 retire — reproduce gate always-on.

**Steps:**
1. Unset env `FLOWPILOT_ENABLE_REPRODUCE_GATE` (hoặc set = `false`).
2. Gọi `ReproduceGateEnabled()`.
3. Xác nhận: trả `true` vô điều kiện (flag không còn tác dụng).
4. Xác nhận: `resolveReproducePrompt`/`resolveReproduceAgent` không còn tồn tại trong codebase.

**Expected:** Always-on, legacy degrade paths đã xóa.

---

## 3. P-2b Test Steps (Task-383: Multi-Language Stub-Body Validation & Language Adapters)

### 3.1 Test 1: TestValidateStubBodiesGo

**Mục tiêu:** Xác nhận stub-body whitelist cho Go pass với stub chuẩn.

**Steps:**
1. Fixture Go: `func GetUser(id string) (*User, error) { return nil, errors.New("not implemented") }`.
2. Gọi `ValidateStubBodies("go", symbols)`.
3. Xác nhận: 0 violation, `BodyShape = StubReturnZero`.

**Expected:** Pass.

### 3.2 Test 2: TestValidateStubBodiesGoRejectsNonStubLogic

**Mục tiêu:** Xác nhận Go body có logic thật bị flag NonStub.

**Steps:**
1. Fixture Go: body chứa `if id == "" { return nil, ErrEmpty }; return db.Query(id)`.
2. Gọi `ValidateStubBodies("go", symbols)`.
3. Xác nhận: violation với symbol + line chỉ đích danh.

**Expected:** Violation NonStub.

### 3.3 Test 3: TestExtractCanonicalSignaturesReactViaNode

**Mục tiêu:** Xác nhận React/TS extraction qua `node` + TypeScript Compiler API (`jsx: true`).

**Steps:**
1. Fixture `.tsx`: function component + arrow function + class method.
2. Gọi `ExtractCanonicalSignatures("Comp.tsx", "react")` (subprocess node chạy `scripts/extract-ts.mjs`).
3. Xác nhận: list symbols đúng name/kind/signature, sorted.

**Expected:** Extraction đúng (nếu `node` không có trong PATH → skip có đánh dấu).

### 3.4 Test 4: TestValidateStubBodiesReactViaNode

**Mục tiêu:** Xác nhận whitelist pass với React stub chuẩn.

**Steps:**
1. Fixture: `function Comp() { throw new Error("not implemented"); }`, `const useHook = () => { throw new Error("not implemented"); }`.
2. Gọi `ValidateStubBodies("react", symbols)`.
3. Xác nhận: 0 violation, `BodyShape = StubThrow`.

**Expected:** Pass.

### 3.5 Test 5: TestValidateStubBodiesReactRejectsNonStubLogic

**Mục tiêu:** Xác nhận React body có JSX/logic thật bị flag.

**Steps:**
1. Fixture: component trả `<div>{data.map(...)}</div>`.
2. Gọi `ValidateStubBodies("react", symbols)`.
3. Xác nhận: violation NonStub.

**Expected:** Violation.

### 3.6 Test 6: TestValidateStubBodiesKotlinViaLSP

**Mục tiêu:** Xác nhận Kotlin body validation qua LSP-anchored (documentSymbol range + foldingRange → cắt body text).

**Steps:**
1. Enable LSP mock cho Kotlin (pattern `TestExtractCanonicalSignaturesKotlinViaLSP`).
2. Fixture: `fun getUser(id: String): User = TODO("not implemented")` + property initializer literal.
3. Gọi `ValidateStubBodies("kotlin", symbols)`.
4. Xác nhận: 0 violation.

**Expected:** Pass.

### 3.7 Test 7: TestExtractCanonicalSignaturesCppViaTreeSitter

**Mục tiêu:** Xác nhận C/C++ extraction qua tree-sitter (build tag `treesitter`).

**Steps:**
1. Build với tag `treesitter` (nếu không → skip có đánh dấu).
2. Fixture `.cpp`: function + method + template function.
3. Gọi `ExtractCanonicalSignatures("service.cpp", "cpp")`.
4. Xác nhận: canonical signature phân biệt overload/template (full qualifier + param types).

**Expected:** Extraction đúng, deterministic.

### 3.8 Test 8: TestValidateStubBodiesCppViaTreeSitter

**Mục tiêu:** Xác nhận C/C++ whitelist pass với stub chuẩn.

**Steps:**
1. Fixture: `int get() { return -1; }`, `User* find() { return nullptr; }`, `void run() { throw std::runtime_error("not implemented"); }`.
2. Gọi `ValidateStubBodies("cpp", symbols)`.
3. Xác nhận: 0 violation.

**Expected:** Pass.

### 3.9 Test 9: TestCppMacroBodyHidingDetected

**Mục tiêu:** Xác nhận macro đa-statement giấu logic bị flag NonStub.

**Steps:**
1. Fixture: stub gọi `RUN_LOGIC(x)`; `#define RUN_LOGIC(x) { if (x) doA(); doB(); }` trong DeclaredPaths.
2. Gọi `ValidateStubBodies("cpp", symbols)`.
3. Xác nhận: violation flag macro đa-statement.

**Expected:** Violation.

### 3.10 Test 10: TestCppDeclDefDedupeSignatureHashStable

**Mục tiêu:** Xác nhận declaration `.h` + definition `.c` dedupe về 1 symbol — hash ổn định.

**Steps:**
1. Fixture: `int foo(int);` trong `service.h`, `int foo(int) { return -1; }` trong `service.c`.
2. Extract cả 2 file, tính `CanonicalSignatureHash`.
3. Xác nhận: 1 symbol duy nhất, hash = hash của definition signature.

**Expected:** Dedupe đúng, hash stable.

### 3.11 Test 11: TestSignatureHashUnchangedWithBodyShapeOutput

**Mục tiêu:** Xác nhận B-8.1 compat — thêm `BodyShape` vào `SymbolInfo` không đổi `SignatureHash`.

**Steps:**
1. Tính hash với extractor P-2 (signature-only).
2. Tính hash với extractor P-2b (signature + BodyShape).
3. Xác nhận: 2 hash giống hệt nhau cho cùng file.

**Expected:** Hash không đổi.

### 3.12 Test 12: TestStubBodyCacheInvalidation

**Mục tiêu:** Xác nhận cache `(path, mtime, content hash)` invalidate đúng.

**Steps:**
1. Parse file → cache populated.
2. Parse lại không đổi → cache hit (không re-parse, verify qua counter).
3. Sửa file (mtime + content đổi) → cache miss → re-parse.

**Expected:** Cache hit/miss đúng.

### 3.13 Test 13: TestStubBodyUnverifiedFailOpen

**Mục tiêu:** Xác nhận fail-open khi parser không khả dụng.

**Steps:**
1. Simulate LSP off + build tag `treesitter` off cho file Kotlin.
2. Gọi `ValidateStubBodies("kotlin", symbols)`.
3. Xác nhận: `BodyShape = Unverified`, KHÔNG violation, evidence `body_unverified` ghi vào gate result.

**Expected:** Fail-open + evidence.

### 3.14 Test 14: TestRuleScaffoldRedFiresOnNonStubBody

**Mục tiêu:** Xác nhận `r-scaffold-red` nhánh (d) — Tín hiệu 3 static.

**Steps:**
1. Setup: scaffold turn với test ĐỎ chuẩn (Tín hiệu 1/2 không fire) NHƯNG `ScaffoldBodyNonStub = true`, `NonStubSymbols = ["GetUser:12"]`.
2. Evaluate rule `r-scaffold-red`.
3. Xác nhận: violation fired, reprompt chỉ đích danh symbol + line.

**Expected:** Violation dù test ĐỎ (đóng lỗ hổng "viết body logic sai nên test vẫn ĐỎ").

### 3.15 Test 15: TestStubBodyAdaptersDispatchByLanguage

**Mục tiêu:** Xác nhận dispatch đúng adapter theo ngôn ngữ + chain fallback.

**Steps:**
1. Gọi với lang `go` → Go native parser.
2. Gọi với lang `react` → node subprocess.
3. Gọi với lang `kotlin` → LSP-anchored.
4. Gọi với lang `cpp` (tag `treesitter`) → tree-sitter; (không tag) → clangd LSP.
5. Xác nhận: mỗi lang dispatch đúng adapter; fallback chain đúng thứ tự.

**Expected:** Dispatch đúng.

---

## 4. P-3 Test Steps (Task-380: Scaffold Architect Prompt & Agent Persona)

### 4.1 Test 1: TestScaffoldArchitectPromptRendering

**Mục tiêu:** Xác nhận prompt `scaffold-contract-tdd.md` render được khi pack load.

**Steps:**
1. Load pack với `scaffold-contract-tdd.md` trong manifest.
2. Render prompt cho node `scaffold_tdd`.
3. Xác nhận:
   - Không lỗi parse/template.
   - Prompt chứa hướng dẫn tạo stubs (4 nhóm ngôn ngữ: Go, Kotlin, TS/React, C/C++).
   - Prompt chứa hướng dẫn viết test assertions.
   - Prompt chứa hướng dẫn chạy test.
   - Prompt chứa hướng dẫn gọi `submit_scaffold_outcome`.

**Expected:** Prompt render success.

---

### 4.2 Test 2: TestScaffoldBehaviorRegistration

**Mục tiêu:** Xác nhận behavior `agent.scaffold` được đăng ký ở runtime registry.

**Steps:**
1. Get `DefaultBehaviorRegistry()`.
2. Xác nhận:
   - `agent.scaffold` registered (có handler).
   - `IsScaffoldBehavior("agent.scaffold") = true`.
   - `NormalizeBehaviorID("scaffold") = "agent.scaffold"`.
   - `NormalizeBehaviorID("scaffold_tdd") = "agent.scaffold"`.

**Expected:** Behavior resolved.

---

### 4.3 Test 3: TestScaffoldNodeAllowsModelField

**Mục tiêu:** Xác nhận model-allowlist mở cho `agent.scaffold` — node scaffold có thể khai `model:` trong flow YAML.

**Steps:**
1. Tạo `FlowNode` với `Behavior: "agent.scaffold"`, `Model: "claude-sonnet-4-5"`.
2. Validate node qua `ValidateFlowDefinition`.
3. Xác nhận: validate pass (không lỗi "model on non-delegate behavior").

**Expected:** Validate pass.

---

## 5. P-4 Test Steps (Task-381: Coder Scaffold Body Prompt & Batch Contract)

### 5.1 Test 1: TestImplementScaffoldBodyPromptRendering

**Mục tiêu:** Xác nhận prompt `implement-scaffold-body.md` render được.

**Steps:**
1. Load pack với `implement-scaffold-body.md`.
2. Render prompt.
3. Xác nhận:
   - Không lỗi parse.
   - Prompt chứa hướng dẫn fill body code.
   - Prompt chứa hướng dẫn gọi `submit_coder_outcome`.

**Expected:** Prompt render success.

---

### 5.2 Test 2: TestImplementScaffoldBodyPromptContainsBatchContract

**Mục tiêu:** Xác nhận prompt chứa 4 lệnh cấm (post-review B-8.1).

**Steps:**
1. Render prompt `implement-scaffold-body.md`.
2. Xác nhận prompt contains:
   - "Cấm sửa file test" (ReadOnly).
   - "Cấm sửa signature" (gate `r-signature-lock`).
   - "Cấm viết thêm test mới".
   - "Cấm thêm function mới, cấm xóa function".
   - Quy tắc Accumulate & Batch.
   - `submit_coder_outcome` status `completed` và `renegotiate_signatures`.

**Expected:** Tất cả 4 lệnh cấm + batch contract có trong prompt.

---

## 6. P-5 Test Steps (Task-382: Flow Topology Scaffold & Renegotiation Loop)

### 6.1 Test 1: TestTaskHarnessTopologyScaffoldNegotiationLoop

**Mục tiêu:** Xác nhận `task-harness.yaml` topology đúng với `synthesis_negotiation` node.

**Steps:**
1. Load pack với `task-harness.yaml` đã sửa.
2. Validate flow topology.
3. Xác nhận:
   - Node `test_signatures` (id giữ nguyên) có behavior `agent.scaffold`, agent `agents/scaffold-architect.md`, promptTemplate `prompts/scaffold-contract-tdd.md`, lifecycle `reinvoke`.
   - Node `implement` có promptTemplate `prompts/implement-scaffold-body.md`.
   - Có node `synthesis_negotiation` (inline hub).
   - Edges: `synthesis → synthesis_negotiation`, `synthesis_negotiation → test_signatures` (back), `synthesis_negotiation → synthesis` (forward).
   - `policy.negotiationCap: 5`.
   - `ValidateFlowSafetyTopology` pass (test_signatures là writer, dominated by contract.freeze).

**Expected:** Topology valid.

---

### 6.2 Test 2: TestVibeSprintTopologyScaffoldNegotiationLoop

**Mục tiêu:** Xác nhận `vibe-sprint.yaml` topology đúng với `synthesis_negotiation` node.

**Steps:**
1. Load pack với `vibe-sprint.yaml` đã sửa.
2. Validate flow topology.
3. Xác nhận:
   - Node `tdd` (id giữ nguyên) có behavior `agent.scaffold`, agent `agents/scaffold-architect.md`, promptTemplate `prompts/scaffold-contract-tdd.md`, lifecycle `reinvoke`, `model: claude-sonnet-4-5`.
   - Node `coder` có promptTemplate `prompts/implement-scaffold-body.md`.
   - Có node `synthesis_negotiation` (inline hub).
   - Edges: `coder → synthesis_negotiation`, `synthesis_negotiation → tdd` (back), `synthesis_negotiation → synthesis` (forward).
   - **Edge `synthesis → coder (when:continue, kind:back)` vẫn giữ** (review loop).
   - `policy.negotiationCap: 5`.
   - `ValidateFlowSafetyTopology` pass.

**Expected:** Topology valid.

---

### 6.3 Test 3: TestScaffoldNegotiationCapEnforcedAtFive

**Mục tiêu:** Xác nhận `negotiationCap: 5` được enforce.

**Steps:**
1. Simulate negotiation loop với `negotiationCap: 5`.
2. Increment `NegotiationRound` 5 lần.
3. Xác nhận:
   - Sau 5 round: `NegotiationRound = 5`, `NegotiationRound >= negotiationCap` → escalate (không extend).
   - Sau 1 round negotiation thành công (synthesis_negotiation → synthesis done): `NegotiationRound` reset về 0.

**Expected:** Cap enforce, reset đúng.

---

### 6.4 Test 4: TestSynthesisNegotiationNodeRouting

**Mục tiêu:** Xác nhận `synthesis_negotiation` node routing đúng.

**Steps:**
1. Simulate hub `synthesis_negotiation` nhận payload batch renegotiation từ coder.
2. Xác nhận:
   - Hub thẩm định batch.
   - Nếu batch hợp lệ: route back-edge `synthesis_negotiation → test_signatures/tdd` (when: continue).
   - Nếu negotiation xong: route forward `synthesis_negotiation → synthesis` (when: done).
   - Sử dụng `SubmitFlowControl` bridge.

**Expected:** Routing đúng.

---

### 6.5 Test 5: TestScaffoldTestLockAndSignatureSnapshotSingleVersion

**Mục tiêu:** Xác nhận `LockScaffoldArtifacts` ghi gộp 1 version bump.

**Steps:**
1. Setup: `FrozenContractRecord` existing với `Version = 1`.
2. Gọi `LockScaffoldArtifacts(store, existing, testPaths=["service_test.go"], signatureHash="abc123", lockedSignatures=["func GetUser(id string) (*User, error)"], now)`.
3. Xác nhận:
   - Record mới có `Version = 2` (1 version bump).
   - `ReadOnlyPaths` chứa `service_test.go`.
   - `SignatureHash = "abc123"`.
   - `LockedSignatures` chứa `func GetUser(...)`.

**Expected:** Record mới với 1 version bump.

---

### 6.6 Test 6: TestLegacyTestSignaturesFlowUnaffected

**Mục tiêu:** Xác nhận `bug-harness`, `rag-harness`, `bug-plan-harness` không bị ảnh hưởng.

**Steps:**
1. Load pack với cả 3 flow.
2. Validate mỗi flow.
3. Xác nhận:
   - `bug-harness.yaml`: topology không đổi, `reproduce_test` node vẫn `agent.reproduce`.
   - `rag-harness.yaml`: topology không đổi, `test_signatures` node vẫn `agent.code` (legacy).
   - `bug-plan-harness.yaml`: topology không đổi.
   - `ValidateFlowSafetyTopology` pass cho tất cả.

**Expected:** 3 flow không đổi, validate pass.

---

## 7. Verification Commands

```bash
# P-1: Tool faces
cd apps/local-runner
go test ./internal/agentpack/... -run 'TestSubmitScaffoldOutcome|TestSubmitCoderOutcome|TestScaffoldOutcomeTool|TestCoderOutcomeTool|TestCoderOutcomeChild' -v

# P-2: Signature lock + scaffold-red
go test ./internal/flowgate/... -run 'TestExtractCanonical|TestCanonicalSignatureHash|TestRuleSignatureLock|TestRuleScaffoldRed|TestLockScaffoldArtifacts|TestReproduceGateRetired' -v

# P-2b: Multi-language stub-body validation (B-11 / Task-383)
go test ./internal/flowgate/... -run 'TestValidateStubBodies|TestStubBody|TestCpp|TestExtractCanonicalSignaturesReact|TestSignatureHashUnchanged' -v

# P-3: Scaffold architect behavior
go test ./internal/agentpack/... -run 'TestScaffoldArchitectPrompt|TestScaffoldBehaviorRegistration|TestScaffoldNodeAllowsModel' -v

# P-4: Coder prompt
go test ./internal/agentpack/... -run 'TestImplementScaffoldBodyPrompt' -v

# P-5: Flow topology
go test ./internal/agentpack/flow-pack/flows/... -run 'TestTaskHarnessTopology|TestVibeSprintTopology|TestScaffoldNegotiationCap|TestSynthesisNegotiation|TestScaffoldTestLock|TestLegacyTestSignatures' -v

# Impact check: legacy flows không đổi
go test ./internal/agentpack/flow-pack/flows/... -run 'TestBugHarness|TestRagHarness|TestBugPlanHarness' -v

# Pack validation (full)
go test ./internal/agentpack/... -run 'TestValidateFlowDefinition|TestLoadBuiltinPack' -v
```

---

## 8. Live Evidence (sau khi implement)

| Test | Status | Evidence |
|------|--------|----------|
| TestSubmitScaffoldOutcomeSchemaValidation | todo | |
| TestSubmitCoderOutcomeBatchSignatureValidation | todo | |
| TestScaffoldOutcomeToolAdvertisedOnScaffoldNode | todo | |
| TestCoderOutcomeToolAdvertisedOnImplementNode | todo | |
| TestCoderOutcomeChildCallIsRecordOnlyAndBuffered | todo | |
| TestExtractCanonicalSignaturesGo | todo | |
| TestCanonicalSignatureHashDeterministic | todo | |
| TestExtractCanonicalSignaturesKotlinViaLSP | todo | |
| TestRuleSignatureLockFailsOnSignatureModification | todo | |
| TestRuleSignatureLockFailsOnAdditiveFunction | todo | |
| TestRuleSignatureLockPassesWhenOnlyBodyModified | todo | |
| TestRuleSignatureLockBypassesWhenRenegotiating | todo | |
| TestRuleScaffoldRedRequiresFailingTests | todo | |
| TestRuleScaffoldRedRejectsCompileFailure | todo | |
| TestRuleScaffoldRedRejectsAllGreen | todo | |
| TestRuleScaffoldRedSuppressesTestRules | todo | |
| TestLockScaffoldArtifactsSingleVersionBump | todo | |
| TestRuleScaffoldRedDetectsBodyLogicAsGreenTests | todo | |
| TestRuleScaffoldRedForcesStubAfterBodyViolation | todo | |
| TestReproduceGateRetiredAlwaysOn | todo | |
| TestValidateStubBodiesGo | todo | |
| TestValidateStubBodiesGoRejectsNonStubLogic | todo | |
| TestExtractCanonicalSignaturesReactViaNode | todo | |
| TestValidateStubBodiesReactViaNode | todo | |
| TestValidateStubBodiesReactRejectsNonStubLogic | todo | |
| TestValidateStubBodiesKotlinViaLSP | todo | |
| TestExtractCanonicalSignaturesCppViaTreeSitter | todo | |
| TestValidateStubBodiesCppViaTreeSitter | todo | |
| TestCppMacroBodyHidingDetected | todo | |
| TestCppDeclDefDedupeSignatureHashStable | todo | |
| TestSignatureHashUnchangedWithBodyShapeOutput | todo | |
| TestStubBodyCacheInvalidation | todo | |
| TestStubBodyUnverifiedFailOpen | todo | |
| TestRuleScaffoldRedFiresOnNonStubBody | todo | |
| TestStubBodyAdaptersDispatchByLanguage | todo | |
| TestScaffoldArchitectPromptRendering | todo | |
| TestScaffoldBehaviorRegistration | todo | |
| TestScaffoldNodeAllowsModelField | todo | |
| TestImplementScaffoldBodyPromptRendering | todo | |
| TestImplementScaffoldBodyPromptContainsBatchContract | todo | |
| TestTaskHarnessTopologyScaffoldNegotiationLoop | todo | |
| TestVibeSprintTopologyScaffoldNegotiationLoop | todo | |
| TestScaffoldNegotiationCapEnforcedAtFive | todo | |
| TestSynthesisNegotiationNodeRouting | todo | |
| TestScaffoldTestLockAndSignatureSnapshotSingleVersion | todo | |
| TestLegacyTestSignaturesFlowUnaffected | todo | |

**Tổng: 46 test signatures**

---

## 9. Change Audit (CA) Notes

Mỗi slice cần 1 CA note (pattern CA-881..CA-884 của CP-66):

- **CA-xxx P-1 (Task-378):** Declared Face Tool Schemas `submit_scaffold_outcome` + `submit_coder_outcome`.
- **CA-xxx P-2 (Task-379):** Signature Lock Rule `r-signature-lock` + `r-scaffold-red` + AST Extractor.
- **CA-xxx P-2b (Task-383):** Multi-Language Stub-Body Validation & Language Adapters (B-11).
- **CA-xxx P-3 (Task-380):** Scaffold Architect Prompt + Agent Persona + Behavior `agent.scaffold`.
- **CA-xxx P-4 (Task-381):** Coder Scaffold Body Prompt & Batch Contract.
- **CA-xxx P-5 (Task-382):** Flow Topology Scaffold & Renegotiation Loop.
- **CA-xxx P-6:** Doc-sync & verification & closeout.

---

## 10. Upstream Doc Sync Checklist

- [ ] `SS-14-Code-Context-And-Regression-Safety.md`: thêm AC-21, update Feature Keys, Child Documents, Last Updated.
- [ ] `SS-19-Engineering-Harness-Flow-Family.md`: siết AC-2 (TDD mandatory → stubs + executable RED tests), thêm AC-9, update Feature Keys, Child Documents, Last Updated.
- [ ] `SS-18-Vibe-Working-Mode.md`: cập nhật TDD-mandatory bullet.
- [ ] `SD-24-Vibe-Working-Mode.md`: cập nhật `tdd` node semantics, thêm `synthesis_negotiation` vào topology sketch.
- [ ] `SP-06-Oracle-Rule-And-Schema-First-Gate.md`: thêm `r-scaffold-red`, `r-signature-lock` vào precedence chain.
- [ ] `SD-20-Flow-Gate-Rule-Semantics.md`: thêm §2.10 `r-signature-lock`, §2.11 `r-scaffold-red` (gồm tín hiệu 3 — static stub-body whitelist, B-11/Task-383), cập nhật §6 traceability, §7 precedence.
- [ ] `CP-64-Reproduce-First-TDD-Gate.md` (done): ghi chú retire flag `FLOWPILOT_ENABLE_REPRODUCE_GATE`.
