# Task-379: Signature Lock Rule & AST Canonical Hash Extractor

## Metadata

- Document ID: `Task-379`
- Title: `Signature Lock Rule & AST Canonical Hash Extractor`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
- Parent Documents: [CP-67 P-2](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [CP-64](../../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-63](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md), [Task-378](./Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-383](./Task-383-Multi-Language-Stub-Body-Validation.md)
- Replaces: `None`
- Tags: `contract-first-tdd, signature-lock, flowgate, ast-hash, gate-hook`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice 2 của CP-67 (P-2): xây dựng 2 gate rules trong flowgate engine:
    - `r-signature-lock`: chặn đứng mọi nỗ lực của Coder sửa đổi API signatures (tên hàm, kiểu tham số, kiểu trả về) — **Canonical Signature-Only Hash (post-review B-8.1)**: hash tính từ **CHỈ signature (function name, params, return types — không bao gồm thân hàm `{ body }`)**. Extractor quét toàn bộ file (full-file scan) để bắt tất cả symbol declarations nhưng **loại bỏ hoàn toàn body**; mọi thay đổi (sửa signature cũ, thêm function mới, xóa function) đều làm hash lệch và phải đi qua batch renegotiation, không có "additive change hợp lệ".
  - `r-scaffold-red` (post-review B-4): enforce compile OK + RED test cho scaffold turn; suppress `r-tests/r-reg` cho turn đó; ghi evidence red tests từ `test_suite.red_tests`.
- Bộ trích xuất chữ ký chuẩn hóa (Canonical AST Signature Extractor):
  - Go: `go/parser` để bóc tách khai báo hàm/struct/interface (loại bỏ thân hàm).
  - Kotlin/TS: **LSP `DocumentSymbols` từ CP-63** — `lsp_client.go` thêm method `DocumentSymbols(filePath)` (post-review B-8.4); fallback regex normalizer khi LSP không khả dụng.
  - Sắp xếp alphabet, băm SHA256 → `SignatureHash`.
- Mở rộng `FrozenContractRecord` với 2 trường mới: `SignatureHash string` (SHA256 canonical) và `LockedSignatures []string` (danh sách chữ ký human-readable để diff khi vi phạm).
- `TurnResult` mở rộng 4 fields: `SignatureHashBefore`, `SignatureHashAfter`, `CoderRenegotiating`, `ScaffoldExpected`, `ScaffoldCompileFailed` (json omitempty, caller-computed).
- `AgentLoopState` mở rộng 2 fields: `NegotiationRound`, `NegotiationCap`.
- Rule không nằm trong `DefaultRules()` (pattern giống `r-reproduce`): gate hook chỉ append khi flow node đang ở bước `implement` của task/vibe flow có scaffold TDD trước đó.
- **Post-review B-8.3**: `LockScaffoldArtifacts` single-write — ghi gộp lock test paths + signature hash + locked signatures vào record của step `implement`/`coder` trong 1 version bump.
- **Post-review B-9**: retire `FLOWPILOT_ENABLE_REPRODUCE_GATE` flag — reproduce gate always-on; legacy degrade paths xóa.

### Current Ask

- Implement P-2 theo CP-67 §4: `signature_lock_rule.go` (new), `scaffold_red_rule.go` (new), `ast_signatures.go` (new), mở rộng `FrozenContractRecord`, wiring trong `gate_hook.go`, 15 test signatures phải xanh.

### Key Decisions

- `T-1` Rule `r-signature-lock` theo pattern `reproduce_rule.go`: builder riêng, không đưa vào `DefaultRules()`, append có chọn lọc khi node implement được preceded bởi scaffold_tdd node có `SignatureHash` khác rỗng trong FrozenContractRecord.
- `T-1b` (post-review B-8.1) **Signature hash (chỉ signature, không bao gồm body)**: hash tính từ CHỈ signature (function name, params, return types — không bao gồm body). Mọi thay đổi — sửa signature cũ, thêm function mới, xóa function — đều làm hash lệch. Không có ngoại lệ "additive change hợp lệ". Nếu Coder thêm helper function → hash lệch → gate chặn trừ khi Coder nộp batch renegotiation.
- `T-1c` (post-review B-4) **Rule `r-scaffold-red`**: enforce compile OK + RED test cho scaffold turn; suppress `r-tests/r-reg` cho turn đó; ghi evidence red tests từ `test_suite.red_tests`. **Detect AI viết body logic thực tế**: nếu `Tests.Failed == 0` (test GREEN) hoặc `ScaffoldCompileFailed` → reprompt yêu cầu AI viết lại thành stub rỗng (signature only). Pattern giống `r-reproduce` (không trong DefaultRules, append có chọn lọc).
- `T-2` Trigger `signature_modified`, Action `reprompt` — Coder vi phạm signature → reprompt với Detail chỉ đích danh hàm nào bị sửa (diff danh sách `LockedSignatures` vs danh sách hiện tại); nếu Coder đồng thời nộp `renegotiate_signatures` qua `submit_coder_outcome` (status=continue) thì violation không fire (renegotiation là hợp pháp, Main Agent sẽ xử lý).
- `T-3` `ast_signatures.go` + `lsp_client.go`: function `ExtractCanonicalSignatures(filePath string, lang string) ([]SymbolInfo, error)` với `SymbolInfo{Name, Kind, Signature, Line}` (Task-383/B-11 bổ sung `BodyShape` vào cùng struct — một lần walk, hai đầu ra) — Go: `go/parser` + `go/ast` (bóc tách FuncDecl, TypeSpec interface/struct); Kotlin/TS: **LSP `DocumentSymbols`** từ CP-63 — `lsp_client.go` thêm method `DocumentSymbols(filePath)` gọi `textDocument/documentSymbol`, fallback regex khi LSP off (post-review B-8.4). `CanonicalSignatureHash` — signature hash (chỉ signature, không bao gồm body): mọi thêm/xóa/sửa signature đều đổi hash (B-8.1).
- `T-4` `CanonicalSignatureHash(signatures []string) string` — sort slice alphabet, join `\n`, SHA256 hex encode; deterministic cho cùng tập signatures bất kể thứ tự khai báo trong file.
- `T-5` `TurnResult` nhận signal `SignatureHashBefore string` + `SignatureHashAfter string` (caller-computed bởi gate hook trước/sau turn Coder); flowgate evaluate so sánh 2 hash.
- `T-6` FrozenContractRecord thêm `SignatureHash string` (json `signature_hash,omitempty`) + `LockedSignatures []string` (json `locked_signatures,omitempty`) — zero-value compatible, pre-CP-67 records không có field → gate bỏ qua check.

### Constraints

- Additive tests only — không edit pre-existing tests.
- Không sửa `reproduce_rule.go` — CP-64 rule set phải byte-stable.
- Go AST extraction phải handle: exported + unexported functions, methods with receiver, interface declarations, struct type declarations.
- Provider parity: rule + extractor không nhận providerKey, provider-agnostic.
- GitNexus impact analysis trước khi sửa `gate_hook.go` và `preflight.go`.

### Open Questions

- `Q-1` (resolved — B-8.4): LSP `documentSymbol` đã nằm trong scope P-2 qua `lsp_client.go` (`DocumentSymbols`); regex chỉ còn là fallback khi LSP off. Câu hỏi parser C/C++/React (tree-sitter cgo vs clangd LSP, node availability) chuyển sang Task-383 (B-11).

### Source Refs

- CP-67 §3.2, §4 P-2.
- `internal/flowgate/reproduce_rule.go` (pattern selective-append, không DefaultRules).
- `internal/flowgate/rules.go` (TurnResult struct, Rule struct).
- `internal/changecontract/preflight.go` (FrozenContractRecord).
- `internal/runner/gate_hook.go` (runFlowGateAtEpoch wiring).
- Go stdlib: `go/parser`, `go/ast`, `crypto/sha256`.

## 1. Goal

Cổng `r-signature-lock` hoạt động trong flowgate engine: turn Coder sửa signature (hash lệch) bị chặn bằng reprompt chỉ đích danh hàm vi phạm; turn Coder chỉ sửa body (hash khớp) thì pass; turn kèm renegotiate_signatures thì bypass check (Main Agent xử lý ở tầng flow). Bộ AST extractor trích xuất deterministic cho Go files.

## 2. Parent Links

- coding plan: `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md` P-2
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

P-1 (Task-378) đã có tool schemas cho TDD và Coder báo cáo; nhưng chưa có cơ chế kiểm duyệt vật lý ngăn Coder sửa lén signature. Slice này tạo hạ tầng gate + extractor mà P-3/P-4/P-5 sẽ wire vào prompt và flow topology.

## 4. Exact Change

- `T-1` **`internal/flowgate/signature_lock_rule.go`** (new): `SignatureLockRuleID = "r-signature-lock"`; `SignatureLockRule() Rule` (Scope `step`, Trigger `signature_modified`, RequiredOutput `signatures_unchanged`, Action `reprompt`). Handler: khi `SignatureHashBefore != "" && SignatureHashAfter != ""` — (a) hash khớp → nil (pass); (b) hash lệch VÀ Coder status != renegotiate_signatures → violation reprompt với diff details; (c) hash lệch VÀ Coder status == renegotiate_signatures → nil (bypass, Main Agent sẽ mediate).
- `T-2` **`internal/flowgate/ast_signatures.go`** (new): `ExtractCanonicalSignatures(filePath, lang string) ([]SymbolInfo, error)` — Go: parse file với `go/parser.ParseFile`, walk AST, collect `FuncDecl` (name + receiver + params + results), `TypeSpec` (interface methods, struct name + fields); Kotlin/TS: **LSP `DocumentSymbols`** primary (B-8.4), regex extractor (bóc `fun`/`function` lines, strip body) khi LSP off. `CanonicalSignatureHash(sigs []string) string` — sort, join, SHA256 hex. `SymbolInfo{Name, Kind, Signature, Line}` — Task-383 (B-11) bổ sung `BodyShape` vào cùng struct.
- `T-2b` **`internal/flowgate/scaffold_red_rule.go`** (new, B-4): `ScaffoldRedRuleID = "r-scaffold-red"`, Scope `step`, Trigger `scaffold_not_red`, Action `reprompt`. Enforce scaffold turn `Tests.Ran && !ScaffoldCompileFailed && len(Tests.Failed) > 0`; suppress `r-tests`/`r-reg` cho turn đó; ghi evidence red tests từ `test_suite.red_tests`. Reprompt chỉ đích danh nguyên nhân: all-green (Tín hiệu 1 — detect body logic), compile-fail (Tín hiệu 2). (Tín hiệu 3 — static stub-body whitelist — do Task-383 wire thêm qua `ScaffoldBodyNonStub`.)
- `T-3` **`internal/flowgate/rules.go`** (modified): thêm **5 fields** vào `TurnResult` (json omitempty, caller-computed): `SignatureHashBefore string`, `SignatureHashAfter string`, `CoderRenegotiating bool`, `ScaffoldExpected bool`, `ScaffoldCompileFailed bool`. (Task-383 bổ sung thêm `ScaffoldBodyNonStub bool` + `NonStubSymbols []string`.)
- `T-4` **`internal/changecontract/preflight.go`** (modified): thêm `SignatureHash string` (json `signature_hash,omitempty`) + `LockedSignatures []string` (json `locked_signatures,omitempty`) vào `FrozenContractRecord`.
- `T-4b` **`internal/runner/lsp_client.go`** (modified, B-8.4): thêm method `DocumentSymbols(filePath) []DocumentSymbol` gọi `textDocument/documentSymbol` (Kotlin/TS signature extraction + Task-383 body anchoring dùng chung).
- `T-4c` **`internal/changecontract/frozen_scope.go`** (modified, B-8.3): `LockScaffoldArtifacts(store, existing, testPaths []string, signatureHash string, lockedSignatures []string, now)` — single-write: khóa test paths vào `ReadOnlyPaths` + ghi `SignatureHash` + `LockedSignatures` vào record step `implement`/`coder` trong **1 version bump**.
- `T-5` **`internal/runner/gate_hook.go`** (modified, B-8.3 + B-9):
  - Scaffold turn pass `r-scaffold-red` → gọi `LockScaffoldArtifacts` (single-write: lock test paths + signature hash + locked signatures vào record của step `implement`/`coder` trong 1 version bump).
  - Coder turn: populate `SignatureHashBefore` từ record, extract lại signatures từ declared paths sau turn, populate `SignatureHashAfter`, append `SignatureLockRule()` vào rule set.
  - Populate `CoderRenegotiating` từ payload `submit_coder_outcome.status == renegotiate_signatures` (runner buffer được).
  - **Post-review B-9**: retire `FLOWPILOT_ENABLE_REPRODUCE_GATE` flag — `ReproduceGateEnabled()` luôn true; legacy degrade paths (`resolveReproducePrompt`/`resolveReproduceAgent`) xóa.
- `T-5b` **`internal/runner/reproduce_gate.go`** (modified, B-9): retire flag — `ReproduceGateEnabled()` trả true vô điều kiện; xóa legacy degrade; generalize lock predicate `IsReproduceBehavior || IsScaffoldBehavior`.
- `T-6` **`internal/flowgate/signature_lock_rule_test.go`** (new) + **`internal/flowgate/ast_signatures_test.go`** (new) + **`internal/flowgate/scaffold_red_rule_test.go`** (new): **15 test signatures**:
  - `TestExtractCanonicalSignaturesGo`
  - `TestCanonicalSignatureHashDeterministic`
  - `TestExtractCanonicalSignaturesKotlinViaLSP`
  - `TestRuleSignatureLockFailsOnSignatureModification`
  - `TestRuleSignatureLockFailsOnAdditiveFunction`
  - `TestRuleSignatureLockPassesWhenOnlyBodyModified`
  - `TestRuleSignatureLockBypassesWhenRenegotiating`
  - `TestRuleScaffoldRedRequiresFailingTests`
  - `TestRuleScaffoldRedRejectsCompileFailure`
  - `TestRuleScaffoldRedRejectsAllGreen`
  - `TestRuleScaffoldRedSuppressesTestRules`
  - `TestLockScaffoldArtifactsSingleVersionBump`
  - `TestRuleScaffoldRedDetectsBodyLogicAsGreenTests`
  - `TestRuleScaffoldRedForcesStubAfterBodyViolation`
  - `TestReproduceGateRetiredAlwaysOn`

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/flowgate/signature_lock_rule.go` (new)
  - `apps/local-runner/internal/flowgate/signature_lock_rule_test.go` (new)
  - `apps/local-runner/internal/flowgate/ast_signatures.go` (new)
  - `apps/local-runner/internal/flowgate/ast_signatures_test.go` (new)
  - `apps/local-runner/internal/flowgate/scaffold_red_rule.go` (new — B-4)
  - `apps/local-runner/internal/flowgate/scaffold_red_rule_test.go` (new — B-4)
  - `apps/local-runner/internal/flowgate/rules.go` (modified — TurnResult signals)
  - `apps/local-runner/internal/changecontract/preflight.go` (modified — 2 new fields)
  - `apps/local-runner/internal/changecontract/frozen_scope.go` (modified — LockScaffoldArtifacts, B-8.3)
  - `apps/local-runner/internal/runner/lsp_client.go` (modified — DocumentSymbols, B-8.4)
  - `apps/local-runner/internal/runner/reproduce_gate.go` (modified — retire flag, B-9)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified — wiring)
- modules: `flowgate`, `changecontract`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Turn coder với `SignatureHashBefore == SignatureHashAfter` (chỉ sửa body) → `r-signature-lock` KHÔNG violation.
- [ ] AC-2: Turn coder với `SignatureHashBefore != SignatureHashAfter` VÀ `CoderRenegotiating=false` → violation reprompt chỉ đích danh signatures bị sửa.
- [ ] AC-3: Turn coder với `SignatureHashBefore != SignatureHashAfter` VÀ `CoderRenegotiating=true` → KHÔNG violation (bypass cho renegotiation flow).
- [ ] AC-3b (post-review B-8.1): Turn coder tự thêm function helper mới (không sửa signature cũ) → `SignatureHashBefore != SignatureHashAfter` → `r-signature-lock` VIOLATION (không bypass, không có "additive change hợp lệ").
- [ ] AC-4: Node không phải implement hoặc FrozenContractRecord không có `SignatureHash` → rule không append, evaluate output không đổi.
- [ ] AC-5: `ExtractCanonicalSignatures` cho Go file trả về deterministic sorted list (function name + receiver + params + return types).
- [ ] AC-5b (post-review B-8.4): `ExtractCanonicalSignatures` cho Kotlin/TS file trả về deterministic list qua LSP `DocumentSymbols` (fallback regex khi LSP off).
- [ ] AC-6: `CanonicalSignatureHash` cho cùng tập signatures (bất kể thứ tự khai báo) → luôn ra cùng SHA256.
- [ ] AC-7: FrozenContractRecord cũ (pre-CP-67, không có `signature_hash`) → marshal/unmarshal byte-identical, gate bỏ qua check.
- [ ] AC-8 (post-review B-4): `r-scaffold-red` enforce compile OK + RED test; suppress `r-tests/r-reg` cho scaffold turn; ghi evidence red tests từ `test_suite.red_tests`.
- [ ] AC-8c (post-review B-4): Detect AI viết body logic thực tế: nếu `Tests.Failed == 0` (test GREEN) hoặc `ScaffoldCompileFailed` → rule `r-scaffold-red` fire reprompt yêu cầu AI viết lại thành stub rỗng (signature only).
- [ ] AC-8b (post-review B-8.3): `LockScaffoldArtifacts` single-write — ghi gộp lock test paths + signature hash + locked signatures vào record step `implement`/`coder` trong 1 version bump.
- [ ] AC-9 (post-review B-9): `FLOWPILOT_ENABLE_REPRODUCE_GATE` flag retire — `ReproduceGateEnabled()` luôn true; legacy degrade paths xóa.
- [ ] AC-10: 15 test signatures green (xem T-6).

## 7. Out of Scope

- LSP documentSymbol integration cho Kotlin/TS (follow-up sau CP-67).
- Tool face schemas (P-1 / Task-378).
- Prompt templates (P-3 / Task-380, P-4 / Task-381).
- Flow topology wiring (P-5 / Task-382).

## 8. Completion Notes

- result: `todo`
- follow-ups: consumed by Task-380/381/382.
- upstream docs updated: `todo`
- **Post-review notes (B-8.1, B-8.3, B-8.4, B-9)**: signature hash (chỉ signature, không bao gồm body) (mọi thêm/xóa/sửa symbol đều fire), single-write LockScaffoldArtifacts, LSP DocumentSymbols cho Kotlin/TS, retire CP-64 flag.
