# Task-383: Multi-Language Stub-Body Validation & Language Adapters

## Metadata

- Document ID: `Task-383`
- Title: `Multi-Language Stub-Body Validation & Language Adapters`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-19`
- Last Updated: `2026-09-19`
- Parent Documents: [CP-67 P-2b](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [CP-67-note §6 (Amendment B-11)](../../07-Coding-Plan/note/CP-67-note.md), [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [CP-63](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md), [Task-379](./Task-379-Signature-Lock-Rule-AST-Extractor.md), [Task-380](./Task-380-Scaffold-Architect-Prompt-Agent-Persona.md)
- Replaces: `None`
- Tags: `contract-first-tdd, b11, stub-body-whitelist, language-adapters, tree-sitter, flowgate`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice P-2b của CP-67 (post-review B-11): tín hiệu detect thứ 3 cho `r-scaffold-red` — **static stub-body whitelist**. Đóng lỗ hổng "TDD AI viết body logic nhưng logic sai nên test vẫn ĐỎ → pass gate" (tín hiệu ĐỎ là necessary-but-not-sufficient), đồng thời chặn nhánh smuggle 90%-implementation + 1-test-đỏ.
- Một lần AST walk trả về cả signature lẫn `BodyShape` của mỗi symbol (`SymbolInfo{Name, Kind, Signature, Line, BodyShape}`); `CanonicalSignatureHash` giữ nguyên signature-only (B-8.1 compat).
- 4 nhóm ngôn ngữ: **Go** (`go/parser`, exact), **React TS/TSX/JS/JSX** (subprocess `node` + TypeScript Compiler API, `jsx: true`, exact), **Kotlin** (LSP-anchored: `documentSymbol` range + `foldingRange` → cắt body text theo tọa độ, near-exact), **C/C++** (tree-sitter-c/cpp sau build tag `treesitter`; mặc định clangd LSP anchored).

### Current Ask

- Implement P-2b theo CP-67 §4: `stub_bodies.go` + 3 language adapters + cache + mở rộng `SymbolInfo`/`TurnResult`/`scaffold_red_rule.go`, 15 test signatures phải xanh.

### Key Decisions

- `T-1` **Một lần walk, hai đầu ra**: `SymbolInfo` thêm `BodyShape` (`StubTodo | StubThrow | StubReturnZero | NonStub | Unverified`) + `Line int`; `CanonicalSignatureHash` và semantics hash KHÔNG đổi (signature-only, B-8.1). Test chứng minh hash ổn định trước/sau thay đổi struct.
- `T-2` **Whitelist shape-based, không string-based**: body hợp lệ = đúng hình dạng statement cho phép (match AST node, không match chuỗi message). Go: đúng 1 statement `panic(<lit>)` hoặc `return` với mỗi operand chỉ `nil`/zero-literal/`errors.New(<lit>)`/`fmt.Errorf(<lit>)` (message khớp `/not.?implemented/i`) / sentinel var `Err.*NotImplemented`. Kotlin: expression body `= TODO(<lit>?)` hoặc block `throw NotImplementedError(<lit>?)`/`error(<lit>)`. TS/React: `throw new Error(<lit>)` (hoặc TypeError/`NotImplemented*` identifier) / arrow `=> TODO()`. C: `return` zero-sentinel (`0`, `-1`, `NULL`, `0.0`, `false`) hoặc `assert(0 && "not implemented")`. C++: `throw std::runtime_error(...)`/`throw std::logic_error(...)`/`return nullptr`/`return {}` — đơn statement.
- `T-3` **Nơi logic trốn phải cover**: Go (package `var` initializer chỉ zero-literal/`errors.New`, `init()` phải rỗng); Kotlin (`init {}` rỗng, property initializer chỉ literal/`TODO()`, default param chỉ literal, companion chỉ `const val`); TS/React (class field chỉ literal/`null`/`undefined` — field arrow function áp luật body, top-level `const` literal hoặc arrow-throwing; `interface`/`type`/`enum` thuần data cho qua); C/C++ (**macro đa-statement trong DeclaredPaths flag NonStub**, global/static initializer có call flag, lambda body áp luật function, template body trong header áp luật).
- `T-4` **React adapter qua node**: subprocess `node` chạy `internal/flowgate/scripts/extract-ts.mjs` (TypeScript Compiler API `ts.createSourceFile`, `jsx: true`) → JSON `SymbolInfo[]`. `.jsx` không type: signature = name + param list text + arity (signature-lock yếu hơn TS nhưng vẫn chặn thêm/xóa param).
- `T-5` **Kotlin adapter LSP-anchored**: `documentSymbol` range + `foldingRange` (fold region body bắt đầu ngay sau dòng signature) → cắt body text theo tọa độ → shape match. Tốt hơn regex full-file: anchor đúng vị trí hàm, không lẫn comment/string/nested brace.
- `T-6` **C/C++ adapter theo build tag**: `//go:build treesitter` → tree-sitter-c/cpp exact parser (cgo); `//go:build !treesitter` (mặc định, `CGO_ENABLED=0`) → clangd LSP anchored. Decl ở `.h` + def ở `.c` **dedupe theo canonical signature** (1 symbol); thêm `#include` không tính là symbol change; canonical signature đủ dài để phân biệt overload/template (full qualifier + param types + template params).
- `T-7` **Cache + fail-open**: cache parse result theo `(path, mtime, content hash)` — gate chạy mỗi turn nhưng chỉ re-parse file đổi. Parse thành công mà body non-stub → luôn violation; parser không khả dụng (LSP off, node không có, tree-sitter không build, dialect lạ) → `BodyShape = Unverified`, KHÔNG violation, evidence `body_unverified` ghi vào gate result (đối thủ thật của gate là lười/thói quen, không phải kẻ gian chủ động — chặn workflow vì tooling hỏng hại hơn).
- `T-8` **Wire vào rule**: `TurnResult` thêm `ScaffoldBodyNonStub bool` + `NonStubSymbols []string` (json omitempty, caller-computed); `scaffold_red_rule.go` thêm nhánh (d) — violation reprompt chỉ đích danh symbol + line; `gate_hook.go` scaffold turn gọi adapters + populate.
- `T-9` **Rollout theo thứ tự dễ→khó**: Go → React → Kotlin → C/C++; land **sau P-5**, không block flow sống (trước khi vào, gate chạy với 2 tín hiệu ban đầu).

### Constraints

- Default build (`CGO_ENABLED=0`, không tag) phải pass toàn bộ test suite — tests tree-sitter skip có đánh dấu khi tag off; tests node/clangd skip khi toolchain không có trong PATH (skip phải được đánh dấu rõ, không silent).
- Không đổi `CanonicalSignatureHash` semantics — pre-CP-67 records vẫn byte-identical.
- Additive tests only — không sửa pre-existing tests của P-2.
- Provider parity: adapters không nhận providerKey, provider-agnostic.
- GitNexus impact analysis trước khi sửa `rules.go`, `scaffold_red_rule.go`, `gate_hook.go`.

### Open Questions

- `Q-1` Node availability trên runner host: assumption repo React/TS đích chắc chắn có `node` (toolchain của chính project). Nếu runner host khác repo host → cần xác nhận.
- `Q-2` clangd cần `compile_commands.json` cho một số codebase C++ phức tạp — fallback anchored-text không cần; chỉ ảnh hưởng độ chính xác, fail-open chấp nhận.

### Source Refs

- CP-67 §3.2 (tín hiệu 3), §4 P-2b; CP-67-note §6 (Amendment B-11).
- Task-379 (extractor interface, rule pattern, `lsp_client.go` `DocumentSymbols`).
- CP-63 (LSP runtime, warm lifecycle).
- Go stdlib: `go/parser`, `go/ast`, `crypto/sha256`; TypeScript Compiler API; tree-sitter-c/cpp.

## 1. Goal

Static stub-body whitelist hoạt động cho 4 nhóm ngôn ngữ: TDD AI viết body logic (đúng lẫn sai — bất kể test ĐỎ hay XANH) đều bị `r-scaffold-red` bắt qua tín hiệu static; `SignatureHash` không đổi; default build không cgo vẫn chạy trọn vẹn.

## 2. Parent Links

- coding plan: `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md` P-2b
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md` (§2.11 `r-scaffold-red` tín hiệu 3)
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

Post-design review phát hiện `r-scaffold-red` chỉ đọc kết quả chạy test (`len(Tests.Failed) > 0`) — AI viết body nửa vời/logic sai vẫn ĐỎ → pass gate. Task-379 (P-2) không cover vì scope của nó là signature extraction + 2 tín hiệu runtime. Slice này thêm tín hiệu static deterministic, tách thành task riêng để giữ Task-379 review-able và cho phép land sau P-5 không block flow.

## 4. Exact Change

- `T-1` **`internal/flowgate/ast_signatures.go`** (modified): `SymbolInfo` thêm `BodyShape BodyShape` + `Line int`; type `BodyShape int` enum (`StubTodo`, `StubThrow`, `StubReturnZero`, `NonStub`, `Unverified`); `CanonicalSignatureHash(sigs []string)` giữ nguyên — helper `CanonicalStrings([]SymbolInfo) []string` tái sử dụng.
- `T-2` **`internal/flowgate/stub_bodies.go`** (new): `ValidateStubBodies(lang string, symbols []SymbolInfo) []BodyViolation` với `BodyViolation{Symbol, Line, Reason}`; whitelist shape per ngôn ngữ (T-2) + nơi trốn (T-3 của Key Decisions).
- `T-3` **`internal/flowgate/scripts/extract-ts.mjs`** (new) + **`internal/flowgate/lang_adapter_react.go`** (new): subprocess `node extract-ts.mjs <file>` → JSON stdout `SymbolInfo[]`; `jsx: true`; handle `.ts/.tsx/.js/.jsx`; `Unverified` khi node không tồn tại.
- `T-4` **`internal/flowgate/lang_adapter_kotlin.go`** (new): LSP-anchored — `DocumentSymbols` + `foldingRange` qua `lsp_client.go` → body text slice → shape match; fallback regex khi LSP off (`Unverified` cho body check).
- `T-5` **`internal/flowgate/lang_adapter_cpp.go`** (new, build tag `treesitter`): tree-sitter-c/cpp; biến thể `lang_adapter_cpp_lsp.go` (build `!treesitter`) dùng clangd anchored. Dedupe decl-vs-def; macro scan trong DeclaredPaths.
- `T-6` **`internal/flowgate/stub_body_cache.go`** (new): cache `(path, mtime, content hash)` → `[]SymbolInfo`; invalidate khi mtime/content đổi.
- `T-7` **`internal/flowgate/rules.go`** (modified): `TurnResult` thêm `ScaffoldBodyNonStub bool` + `NonStubSymbols []string` (json omitempty).
- `T-8` **`internal/flowgate/scaffold_red_rule.go`** (modified): nhánh (d) `ScaffoldBodyNonStub` → violation reprompt với detail symbol + line + reason (Tín hiệu 3).
- `T-9` **`internal/runner/gate_hook.go`** (modified): scaffold turn — gọi `ExtractCanonicalSignatures` (đã có từ P-2) + `ValidateStubBodies`; populate `ScaffoldBodyNonStub` + `NonStubSymbols`; dispatch theo lang của DeclaredPaths (go/react/kotlin/cpp).
- `T-10` **Tests** (new): **15 test signatures**:
  - `TestValidateStubBodiesGo`
  - `TestValidateStubBodiesGoRejectsNonStubLogic`
  - `TestExtractCanonicalSignaturesReactViaNode`
  - `TestValidateStubBodiesReactViaNode`
  - `TestValidateStubBodiesReactRejectsNonStubLogic`
  - `TestValidateStubBodiesKotlinViaLSP`
  - `TestExtractCanonicalSignaturesCppViaTreeSitter`
  - `TestValidateStubBodiesCppViaTreeSitter`
  - `TestCppMacroBodyHidingDetected`
  - `TestCppDeclDefDedupeSignatureHashStable`
  - `TestSignatureHashUnchangedWithBodyShapeOutput`
  - `TestStubBodyCacheInvalidation`
  - `TestStubBodyUnverifiedFailOpen`
  - `TestRuleScaffoldRedFiresOnNonStubBody`
  - `TestStubBodyAdaptersDispatchByLanguage`

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/flowgate/ast_signatures.go` (modified — SymbolInfo + BodyShape)
  - `apps/local-runner/internal/flowgate/stub_bodies.go` (new)
  - `apps/local-runner/internal/flowgate/stub_bodies_test.go` (new)
  - `apps/local-runner/internal/flowgate/scripts/extract-ts.mjs` (new)
  - `apps/local-runner/internal/flowgate/lang_adapter_react.go` (new)
  - `apps/local-runner/internal/flowgate/lang_adapter_kotlin.go` (new)
  - `apps/local-runner/internal/flowgate/lang_adapter_cpp.go` (new — build tag `treesitter`)
  - `apps/local-runner/internal/flowgate/lang_adapter_cpp_lsp.go` (new — build `!treesitter`)
  - `apps/local-runner/internal/flowgate/stub_body_cache.go` (new)
  - `apps/local-runner/internal/flowgate/rules.go` (modified — 2 TurnResult fields)
  - `apps/local-runner/internal/flowgate/scaffold_red_rule.go` (modified — nhánh d)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified — wiring)
- modules: `flowgate`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Go stub chuẩn pass; body có `if/for/switch`/multi-statement/call user func → violation với symbol + line.
- [ ] AC-2: React/TS stub chuẩn pass qua node; component trả JSX thật → violation. Skip có đánh dấu khi node không có.
- [ ] AC-3: Kotlin stub chuẩn pass qua LSP-anchored; `init {}` có logic / property initializer có call → violation.
- [ ] AC-4: C/C++ stub chuẩn pass; macro đa-statement → violation; decl `.h` + def `.c` dedupe 1 symbol, hash stable.
- [ ] AC-5: `SignatureHash` cho cùng file giống hệt trước và sau khi thêm `BodyShape` (B-8.1 compat).
- [ ] AC-6: Cache hit khi file không đổi; cache miss khi mtime/content đổi.
- [ ] AC-7: Parser không khả dụng → `Unverified`, không violation, evidence `body_unverified`; parse thành công mà non-stub → luôn violation.
- [ ] AC-8: `r-scaffold-red` nhánh (d) fire khi `ScaffoldBodyNonStub=true` dù test ĐỎ chuẩn — reprompt chỉ đích danh symbol + line.
- [ ] AC-9: Dispatch đúng adapter theo lang; fallback chain exact → LSP-anchored → regex → Unverified.
- [ ] AC-10: Default build `CGO_ENABLED=0` pass toàn bộ test suite; tests tree-sitter skip có đánh dấu khi tag off.
- [ ] AC-11: Pre-CP-67 `FrozenContractRecord` (không có `signature_hash`) marshal/unmarshal byte-identical — không regression.
- [ ] AC-12: 15 test signatures green (xem T-10).

## 7. Out of Scope

- Signature hash semantics thay đổi (giữ nguyên B-8.1).
- Test-runner multi-language cho harness (assumption đã có — xem CP-67 Constraints).
- Ad-hoc ngôn ngữ ngoài 4 nhóm (Java thuần, Python...) — thêm sau nếu cần, mỗi ngôn ngữ 1 adapter.
- Renegotiation loop / flow topology (P-5 / Task-382 đã lo).

## 8. Completion Notes

- result: `todo`
- follow-ups: nâng cấp Kotlin adapter sang tree-sitter nếu cần exact; expose body-check coverage metric trong gate evidence.
- upstream docs updated: `todo`
- **Post-review notes (B-11)**: tín hiệu 3 land sau P-5; fail-open mặc định; không cgo default build.
