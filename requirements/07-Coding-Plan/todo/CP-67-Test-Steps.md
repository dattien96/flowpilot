# CP-67 Test Steps & Live Evidence

## Metadata

- Document ID: `CP-67-Test-Steps`
- Title: `CP-67 Contract-First Scaffold TDD: Test Steps & Live Evidence`
- Phase: `coding_plan`
- Status: `done` (2026-09-20 — automated matrix verified: 46/46 signatures green or mapped; §11 live runbook defined, provider live runs pending operator execution — see §11.4)
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

Evidence recorded 2026-09-20 on branch `cp68` (commits `1571f961`..HEAD). Commands per §7; focused CP-67 suite: **101 pass / 1 fail pre-existing** (`TestIsFlowPlannerExcludedPathCoversSkillpackScaffold` — fails identically on clean HEAD `eb2e07f6`, unrelated to CP-67).

| Test | Status | Evidence |
|------|--------|----------|
| TestSubmitScaffoldOutcomeSchemaValidation | pass | `agentpack/scaffold_tool_face_test.go` |
| TestSubmitCoderOutcomeBatchSignatureValidation | pass | `agentpack/scaffold_tool_face_test.go` |
| TestScaffoldOutcomeToolAdvertisedOnScaffoldNode | pass | `agentpack/scaffold_tool_face_test.go` |
| TestCoderOutcomeToolAdvertisedOnImplementNode | pass | `agentpack/scaffold_tool_face_test.go` |
| TestCoderOutcomeChildCallIsRecordOnlyAndBuffered | pass (mapped) | Renamed: `TestSubmitFlowControlCoderBatchIsRecordOnly` + `TestHandleSubmitFlowControlCoderOutcomeHTTP` (`runner/cp67_coder_transport_test.go`) — covers record-only + buffered on both bridge and HTTP paths |
| TestExtractCanonicalSignaturesGo | pass | `flowgate/ast_signatures_test.go` |
| TestCanonicalSignatureHashDeterministic | pass | `flowgate/ast_signatures_test.go` |
| TestExtractCanonicalSignaturesKotlinViaLSP | pass | `flowgate/ast_signatures_test.go` |
| TestRuleSignatureLockFailsOnSignatureModification | pass | `flowgate/signature_lock_rule_test.go` |
| TestRuleSignatureLockFailsOnAdditiveFunction | pass | `flowgate/signature_lock_rule_test.go` |
| TestRuleSignatureLockPassesWhenOnlyBodyModified | pass | `flowgate/signature_lock_rule_test.go` |
| TestRuleSignatureLockBypassesWhenRenegotiating | pass | `flowgate/signature_lock_rule_test.go` |
| TestRuleScaffoldRedRequiresFailingTests | pass | `flowgate/scaffold_red_rule_test.go` |
| TestRuleScaffoldRedRejectsCompileFailure | pass | `flowgate/scaffold_red_rule_test.go` |
| TestRuleScaffoldRedRejectsAllGreen | pass | `flowgate/scaffold_red_rule_test.go` |
| TestRuleScaffoldRedSuppressesTestRules | pass | `flowgate/scaffold_red_rule_test.go` |
| TestLockScaffoldArtifactsSingleVersionBump | pass | `changecontract/scaffold_lock_test.go` |
| TestRuleScaffoldRedDetectsBodyLogicAsGreenTests | pass | `flowgate/scaffold_red_rule_test.go` |
| TestRuleScaffoldRedForcesStubAfterBodyViolation | pass | `flowgate/scaffold_red_rule_test.go` |
| TestReproduceGateRetiredAlwaysOn | pass | `runner/scaffold_behavior_test.go` |
| TestValidateStubBodiesGo | pass | `flowgate/stub_bodies_test.go` |
| TestValidateStubBodiesGoRejectsNonStubLogic | pass | `flowgate/stub_bodies_test.go` |
| TestExtractCanonicalSignaturesReactViaNode | pass | `flowgate/stub_bodies_test.go` |
| TestValidateStubBodiesReactViaNode | pass | `flowgate/stub_bodies_test.go` |
| TestValidateStubBodiesReactRejectsNonStubLogic | pass | `flowgate/stub_bodies_test.go` |
| TestValidateStubBodiesKotlinViaLSP | pass | `flowgate/stub_bodies_test.go` |
| TestExtractCanonicalSignaturesCppViaTreeSitter | pass (mapped) | Renamed: `TestExtractCanonicalSignaturesCppViaLSP` — default build is no-cgo so the C++ path runs through the LSP adapter, not tree-sitter (`flowgate/stub_bodies_test.go`) |
| TestValidateStubBodiesCppViaTreeSitter | pass (mapped) | Renamed: `TestValidateStubBodiesCpp` — same no-cgo/LSP rationale (`flowgate/stub_bodies_test.go`) |
| TestCppMacroBodyHidingDetected | pass | `flowgate/stub_bodies_test.go` |
| TestCppDeclDefDedupeSignatureHashStable | pass | `flowgate/stub_bodies_test.go` |
| TestSignatureHashUnchangedWithBodyShapeOutput | pass | `flowgate/stub_bodies_test.go` |
| TestStubBodyCacheInvalidation | pass | `flowgate/stub_bodies_test.go` |
| TestStubBodyUnverifiedFailOpen | pass | `flowgate/stub_bodies_test.go` |
| TestRuleScaffoldRedFiresOnNonStubBody | pass | `flowgate/scaffold_red_rule_test.go` |
| TestStubBodyAdaptersDispatchByLanguage | pass | `flowgate/stub_bodies_test.go` |
| TestScaffoldArchitectPromptRendering | pass | `agentpack/scaffold_architect_test.go` |
| TestScaffoldBehaviorRegistration | pass | `runner/scaffold_behavior_test.go` |
| TestScaffoldNodeAllowsModelField | pass | `agentpack/scaffold_architect_test.go` |
| TestImplementScaffoldBodyPromptRendering | pass | `agentpack/scaffold_architect_test.go` |
| TestImplementScaffoldBodyPromptContainsBatchContract | pass | `agentpack/scaffold_architect_test.go` |
| TestTaskHarnessTopologyScaffoldNegotiationLoop | pass | `agentpack/scaffold_flow_topology_test.go` |
| TestVibeSprintTopologyScaffoldNegotiationLoop | pass | `agentpack/scaffold_flow_topology_test.go` |
| TestScaffoldNegotiationCapEnforcedAtFive | pass | `runner/scaffold_negotiation_test.go` |
| TestSynthesisNegotiationNodeRouting | pass | `runner/scaffold_negotiation_test.go` |
| TestScaffoldTestLockAndSignatureSnapshotSingleVersion | pass | `runner/scaffold_lock_test.go` |
| TestLegacyTestSignaturesFlowUnaffected | pass | `agentpack/scaffold_flow_topology_test.go` |

**Tổng: 46 test signatures — 43 exact-name pass, 3 pass under mapped names (see above).**

### 8.1 Additional matrix coverage (added post-implementation, CA-898)

Beyond the 46 spec signatures, the transport/dispatch layer carries:

| Test | Status | Evidence |
|------|--------|----------|
| TestReviewOutcomeAcceptsRenegotiateSignaturesAndPreservesBatch | pass | `runner/cp67_coder_transport_test.go` |
| TestReviewOutcomeRejectsRenegotiateWithoutBatch | pass | `runner/cp67_coder_transport_test.go` |
| TestCoderOutcomeFaceMapsDomainStatuses | pass | `runner/cp67_coder_transport_test.go` |
| TestSubmitFlowControlCoderBatchIsRecordOnly | pass | `runner/cp67_coder_transport_test.go` |
| TestSubmitFlowControlDelegateCannotSettleOrLoop | pass | `runner/cp67_coder_transport_test.go` |
| TestHandleSubmitFlowControlCoderOutcomeHTTP | pass | `runner/cp67_coder_transport_test.go` |
| TestHandleSubmitFlowControlRejectsUnknownRun | pass | `runner/cp67_coder_transport_test.go` |
| TestContinueBackEdgePhaseHubDoesNotShadowWriterEdge | pass | `runner/cp67_coder_transport_test.go` |
| TestNegotiationHubNodeForFindsPhaseHub | pass | `runner/cp67_coder_transport_test.go` |
| TestNegotiationPhaseActiveViaActiveHubNodeID | pass | `runner/cp67_coder_transport_test.go` |
| TestRenderNegotiationBatchPromptCarriesRowsAndRouting | pass | `runner/cp67_coder_transport_test.go` |
| TestCoderCompletionDispatchesNegotiationHubWithBatch | pass | `runner/cp67_coder_transport_test.go` |
| TestCoderCompletionWithoutBatchAdvancesNormally | pass | `runner/cp67_coder_transport_test.go` |
| TestIsSignatureLockedCoderChild | pass | `runner/cp67_coder_transport_test.go` |
| TestCoderBatchAccumulatesAcrossSubmissions | pass | `runner/cp67_coder_transport_test.go` — batches accumulate per-run until hub consumes |
| TestNegotiationMultiRoundRedispatchesHub | pass | `runner/cp67_coder_transport_test.go` — round 2 re-buffer → re-dispatch → `NegotiationRound=2` |
| TestCoderOutcomeTransportProviderParity | pass | `runner/cp67_coder_transport_test.go` — identical record-only outcome for claude/codex/grok keys via shared `turnBridge.SubmitFlowControl` |

### 8.2 Known limitations & pre-existing failures

- **Batch buffer is in-memory only** (`interactiveRun.pendingBatchSignatureByStep`, not serialized into `ProviderSessionState`). If the runner service restarts while a coder batch is pending, the buffered rows are lost and coder completion advances on the normal `done` edge instead of dispatching `synthesis_negotiation` — graceful degradation, no hang/crash, but the negotiation round is skipped. The CP-67 contract does not require restart durability; recorded here as explicit residual limitation, not claimed as durable.
- **Pre-existing failures (verified identical on clean HEAD `eb2e07f6`, NOT caused by CP-67):**
  - `TestBUG327_EmitLockedDoesNotUnparkWaitingChild` — self-deadlock: test holds `svc.mu` then calls `agentGraphSnapshot` (which locks `s.mu` since BUG-367, 2026-09-10). Test can never complete; broken ~10 days before CP-67.
  - `TestSyncedChatCanResumeAfterServiceRestart` — `run_not_found`.
  - `TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget` — fails identically on HEAD.
  - `TestIsFlowPlannerExcludedPathCoversSkillpackScaffold` — `docs/report.md` excluded-path check, fails identically on HEAD.

---

## 9. Change Audit (CA) Notes

Mỗi slice cần 1 CA note (pattern CA-881..CA-884 của CP-66). Đã tạo:

- **CA-895** (P-1/P-3/P-4 + topology data): agentpack pack data — declared faces (`1571f961`), scaffold-architect persona + behavior (`ee462737`), coder body prompt (`b916ccde`), negotiation topology (`125eded5`).
- **CA-896** (P-2/P-2b): signature extraction + lock rules + changecontract lock (`f2fcacdd`); stub-body whitelist + language adapters (`38a9c810`).
- **CA-897** (P-5): runner wiring — agent.scaffold behavior + flag retire (`09c6dd94`), coder-outcome transport + record-only batch (`d963a7b6`), scaffold gate wiring + artifact lock (`4b19dd5e`), negotiation dispatch + transport tests (`603585aa`).
- **CA-898** (P-6): closeout — doc-sync verification, evidence table, matrix gap tests, live runbook.

---

## 10. Upstream Doc Sync Checklist

- [x] `SS-14-Code-Context-And-Regression-Safety.md`: thêm AC-21, update Feature Keys, Child Documents, Last Updated. (commit `cfb13940`)
- [x] `SS-19-Engineering-Harness-Flow-Family.md`: siết AC-2 (TDD mandatory → stubs + executable RED tests), thêm AC-9, update Feature Keys, Child Documents, Last Updated. (commit `cfb13940`)
- [x] `SS-18-Vibe-Working-Mode.md`: cập nhật TDD-mandatory bullet. (commit `cfb13940`)
- [x] `SD-24-Vibe-Working-Mode.md`: cập nhật `tdd` node semantics, thêm `synthesis_negotiation` vào topology sketch. (commit `cfb13940`)
- [x] `SP-06-Oracle-Rule-And-Schema-First-Gate.md`: thêm `r-scaffold-red`, `r-signature-lock` vào precedence chain (§82–86). (commit `cfb13940`)
- [x] `SD-20-Flow-Gate-Rule-Semantics.md`: thêm §2.10 `r-signature-lock`, §2.11 `r-scaffold-red` (gồm tín hiệu 3 — static stub-body whitelist, B-11/Task-383), cập nhật §6 traceability, §7 precedence. (commit `cfb13940`)
- [x] `CP-64-Reproduce-First-TDD-Gate.md`: retire note cho `FLOWPILOT_ENABLE_REPRODUCE_GATE` (always-on, B-9).
- [ ] `CP-64-Reproduce-First-TDD-Gate.md` (done): ghi chú retire flag `FLOWPILOT_ENABLE_REPRODUCE_GATE`.

---

## 11. Manual / Live Verification Runbook

Automated evidence (§8) covers the unit/integration matrix. This section is the **live runbook** — real provider turns (grok first), real workspace, real HTTP API. Record results in §11.6; do not mark a row `pass` without a captured run ID + artifact path.

### 11.1 Environment & preflight

| # | Check | Command / observation | Expected |
|---|-------|-----------------------|----------|
| S1 | Runner builds | `cd apps/local-runner && go build -o /tmp/flowpilot ./cmd/flowpilot` | clean build |
| S2 | Runner serves | `/tmp/flowpilot runner serve --port 4317` (cwd = repo root) | health endpoint answers |
| S3 | Provider | `grok` account connected in `provider-accounts.json` (slot 0/3/4) | `auth_status: connected` |
| S4 | Workspace | target = `/Users/tiendat/Desktop/BE/gate-sandbox` — must be readable/writable by the runner process | `ls`/`stat` OK; if EPERM, grant the terminal host Desktop folder access |
| S5 | Diag logging | `export FLOWPILOT_FLOW_DIAG_DIR=<workspace>/.flowpilot/logs/features/agent-flow-engine` (or leave default) | `<runID>.ndjson` appears on first event |

Start a live run:

```bash
curl -s -X POST localhost:4317/client/workflow-runs \
  -H 'content-type: application/json' -d '{
    "projectId": "gate-sandbox",
    "flowRef": "task-harness",
    "providerKey": "grok",
    "model": "grok-4-5",
    "workingMode": "dev",
    "cwd": "/Users/tiendat/Desktop/BE/gate-sandbox"
  }' | tee /tmp/cp67-run.json        # capture runId
curl -s -X POST localhost:4317/client/workflow-runs/$RUN/turns \
  -H 'content-type: application/json' -d '{
    "stepId": "test_signatures",
    "prompt": "<feature brief — see scenario>"
  }'
```

Watch artifacts live:

```bash
tail -f .flowpilot/logs/features/agent-flow-engine/$RUN.ndjson   # flow_diag events
tail -f .flowpilot/gate-metrics.ndjson                            # gate results
cat  .flowpilot/contracts/frozen_contracts.ndjson | jq            # SignatureHash / LockedSignatures / ReadOnlyPaths
```

### 11.2 Scenario L-A — contract-first happy path (`task-harness`)

Brief: *"Implement a `calc` package: `Add(a, b int) (int, error)` returns the sum; error on overflow."*

| # | Checkpoint | How to observe | Expected |
|---|-----------|----------------|----------|
| A1 | scaffold node runs | diag `flow_node_*` / agent-graph `test_signatures` active | Scaffold Architect (grok) turn, `calc/scaffold_test.go` + stub `calc/calc.go` written |
| A2 | suite is compile-OK + RED | `cd <ws> && go test ./calc/` during/after scaffold turn | ≥1 failing test (`not implemented` / assertion) |
| A3 | artifacts locked | `frozen_contracts.ndjson` entry for step | `signature_hash` non-empty, `locked_signatures` rows, `read_only_paths` contains the test file |
| A4 | coder gets body-only prompt | turn prompt in dispatch log / rollout | `implement-scaffold-body.md` text; no test-authoring instructions |
| A5 | signature lock holds | gate-metrics + `go test ./calc/` after coder | green suite; any signature drift → `r-signature-lock` reprompt |
| A6 | review back-edge intact | diag `flow_advance_*` after a `changes_requested` | re-enters `implement`, never `test_signatures` |

### 11.3 Scenario L-B — gate rejections (negative cases)

| # | Case | Trigger | Expected |
|---|------|---------|----------|
| B1 | scaffold writes real logic | brief nudges "implement it fully" | `r-scaffold-red` signal 3 / stub whitelist → reprompt, not pass |
| B2 | scaffold suite all-green | brief asks for trivially-true tests | `r-scaffold-red` rejects: needs ≥1 RED |
| B3 | scaffold suite won't compile | malformed scaffold output | rejected as compile failure, not RED credit |
| B4 | coder edits test file | (manual: attempt edit to read-only test) | rejected — ReadOnlyPaths enforcement |
| B5 | coder drifts a signature | brief incompatible with frozen sig (e.g. extra return) without renegotiation | `r-signature-lock` violation → reprompt |
| B6 | coder adds a new function | brief requires helper beyond frozen sig | additive change → `r-signature-lock` violation |

### 11.4 Scenario L-C — renegotiation loop

Brief deliberately under-specified so the coder must renegotiate (e.g. frozen `Add(a,b int) error` but feature needs the sum returned):

| # | Checkpoint | How to observe | Expected |
|---|-----------|----------------|----------|
| C1 | coder submits `renegotiate_signatures` | POST flow-control response / turn tool result | `renegotiation_recorded`; loop state unchanged (record-only) |
| C2 | batch buffered | `snapshotCoderBatchSignatures` via diag/`flow_advance_*` | rows present pre-completion |
| C3 | hub dispatch on coder done | diag `flow_advance_negotiation_hub` | `completed_node_id=implement`, `batch_rows>0` |
| C4 | batch in hub prompt | hub turn prompt in rollout/dispatch | symbol/file/current/proposed rows verbatim |
| C5 | round increments | `GET .../agent-graph` loop state after hub `continue` | `NegotiationRound=1`, `Round` unchanged |
| C6 | cap at 5 | keep failing adjudication | round-5 `continue` → `blocked`/escalate, never extend |
| C7 | phase close | hub `done` | `NegotiationRound` resets 0; proceeds to `synthesis` |
| C8 | vibe-sprint parity | same flow on `vibe-sprint` | identical behavior via `coder`→`synthesis_negotiation` |

### 11.5 Scenario L-D — regression + edge cases

| # | Case | Expected |
|---|------|----------|
| D1 | `bug-harness`/`bug-plan-harness` run unaffected | legacy reproduce flow intact (no scaffold nodes) |
| D2 | `normal_chat` run unaffected | no gates/locks applied |
| D3 | delegate child bare `done`/`continue` | rejected — cannot settle/loop parent |
| D4 | `renegotiate_signatures` without batch | 400 — `batch_signature_requests` required |
| D5 | flow-control to unknown run | 404 |
| D6 | restart mid-negotiation | batch buffer lost → coder completion advances normally (known in-memory limitation, §8.2) — graceful, no hang |

### 11.6 Live results matrix

| Case | Provider | Flow | Run ID | Result | Evidence (log/artifact path) | Status |
|------|----------|------|--------|--------|------------------------------|--------|
| L-A1..A6 | grok | task-harness | | | | pending |
| L-B1..B6 | grok | task-harness | | | | pending |
| L-C1..C7 | grok | task-harness | | | | pending |
| L-C8 | grok | vibe-sprint | | | | pending |
| L-D1..D6 | grok | mixed | | | | pending |
| L-A spot | codex | task-harness | | | | pending |
| L-A spot | claude | task-harness | | | | pending |

**Live run status (2026-09-20):** runner build verified; grok accounts present (slots 0/3/4 connected). Target workspace `/Users/tiendat/Desktop/BE/gate-sandbox` is currently EPERM-blocked to the agent/terminal process (readdir denied at OS level — needs Desktop folder permission for the host process). Runs pending until access is granted or an alternate workspace is confirmed.
