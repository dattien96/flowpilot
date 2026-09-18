# Task-379: Signature Lock Rule & AST Canonical Hash Extractor

## Metadata

- Document ID: `Task-379`
- Title: `Signature Lock Rule & AST Canonical Hash Extractor`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: [CP-67 P-2](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [CP-64](../../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-63](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md), [Task-378](./Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md)
- Replaces: `None`
- Tags: `contract-first-tdd, signature-lock, flowgate, ast-hash, gate-hook`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice 2 của CP-67: xây dựng gate rule `r-signature-lock` trong flowgate engine — chặn đứng mọi nỗ lực của Coder sửa đổi API signatures (tên hàm, kiểu tham số, kiểu trả về) mà TDD đã thiết kế ở bước trước.
- Bộ trích xuất chữ ký chuẩn hóa (Canonical AST Signature Extractor): parse file Go bằng `go/parser` để bóc tách khai báo hàm/struct/interface (loại bỏ thân hàm), sắp xếp alphabet, băm SHA256 → `SignatureHash`.
- Mở rộng `FrozenContractRecord` với 2 trường mới: `SignatureHash string` (SHA256 canonical) và `LockedSignatures []string` (danh sách chữ ký human-readable để diff khi vi phạm).
- Rule không nằm trong `DefaultRules()` (pattern giống `r-reproduce`): gate hook chỉ append khi flow node đang ở bước `implement` của task/vibe flow có scaffold TDD trước đó.

### Current Ask

- Implement P-2 theo CP-67 §4: `signature_lock_rule.go` (new), `ast_signatures.go` (new), mở rộng `FrozenContractRecord`, wiring trong `gate_hook.go`, 4 test signatures phải xanh.

### Key Decisions

- `T-1` Rule `r-signature-lock` theo pattern `reproduce_rule.go`: builder riêng, không đưa vào `DefaultRules()`, append có chọn lọc khi node implement được preceded bởi scaffold_tdd node có `SignatureHash` khác rỗng trong FrozenContractRecord.
- `T-2` Trigger `signature_modified`, Action `reprompt` — Coder vi phạm signature → reprompt với Detail chỉ đích danh hàm nào bị sửa (diff danh sách `LockedSignatures` vs danh sách hiện tại); nếu Coder đồng thời nộp `renegotiate_signatures` qua `submit_coder_outcome` (status=continue) thì violation không fire (renegotiation là hợp pháp, Main Agent sẽ xử lý).
- `T-3` `ast_signatures.go`: function `ExtractCanonicalSignatures(filePath string, lang string) ([]string, error)` — Go implementation dùng `go/parser` + `go/ast` (bóc tách FuncDecl, TypeSpec interface/struct); Kotlin/TS ban đầu dùng regex normalizer đơn giản (`func/fun/function` + tên + tham số + return type, bỏ body `{...}`); dự kiến nâng cấp lên LSP documentSymbol sau.
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

- `Q-1` (từ CP-67): Kotlin/TS extractor ban đầu dùng regex; nâng cấp lên LSP documentSymbol sẽ là follow-up task riêng sau khi CP-67 đóng.

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
- `T-2` **`internal/flowgate/ast_signatures.go`** (new): `ExtractCanonicalSignatures(filePath, lang string) ([]string, error)` — Go: parse file với `go/parser.ParseFile`, walk AST, collect `FuncDecl` (name + receiver + params + results), `TypeSpec` (interface methods, struct name + fields); Kotlin/TS: regex extractor (bóc `fun`/`function` lines, strip body). `CanonicalSignatureHash(sigs []string) string` — sort, join, SHA256 hex.
- `T-3` **`internal/flowgate/rules.go`** (modified): thêm `SignatureHashBefore string` + `SignatureHashAfter string` + `CoderRenegotiating bool` vào `TurnResult` (json omitempty, caller-computed).
- `T-4` **`internal/changecontract/preflight.go`** (modified): thêm `SignatureHash string` (json `signature_hash,omitempty`) + `LockedSignatures []string` (json `locked_signatures,omitempty`) vào `FrozenContractRecord`.
- `T-5` **`internal/runner/gate_hook.go`** (`runFlowGateAtEpoch`): khi node implement được preceded bởi scaffold_tdd node VÀ FrozenContractRecord có `SignatureHash` khác rỗng — populate `SignatureHashBefore` từ record, extract lại signatures từ declared paths sau turn, populate `SignatureHashAfter`, append `SignatureLockRule()` vào rule set.
- `T-6` **`internal/flowgate/signature_lock_rule_test.go`** (new) + **`internal/flowgate/ast_signatures_test.go`** (new): 4 test signatures.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/flowgate/signature_lock_rule.go` (new)
  - `apps/local-runner/internal/flowgate/signature_lock_rule_test.go` (new)
  - `apps/local-runner/internal/flowgate/ast_signatures.go` (new)
  - `apps/local-runner/internal/flowgate/ast_signatures_test.go` (new)
  - `apps/local-runner/internal/flowgate/rules.go` (modified — TurnResult signals)
  - `apps/local-runner/internal/changecontract/preflight.go` (modified — 2 new fields)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified — wiring)
- modules: `flowgate`, `changecontract`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Turn coder với `SignatureHashBefore == SignatureHashAfter` (chỉ sửa body) → `r-signature-lock` KHÔNG violation.
- [ ] AC-2: Turn coder với `SignatureHashBefore != SignatureHashAfter` VÀ `CoderRenegotiating=false` → violation reprompt chỉ đích danh signatures bị sửa.
- [ ] AC-3: Turn coder với `SignatureHashBefore != SignatureHashAfter` VÀ `CoderRenegotiating=true` → KHÔNG violation (bypass cho renegotiation flow).
- [ ] AC-4: Node không phải implement hoặc FrozenContractRecord không có `SignatureHash` → rule không append, evaluate output không đổi.
- [ ] AC-5: `ExtractCanonicalSignatures` cho Go file trả về deterministic sorted list (function name + receiver + params + return types).
- [ ] AC-6: `CanonicalSignatureHash` cho cùng tập signatures (bất kể thứ tự khai báo) → luôn ra cùng SHA256.
- [ ] AC-7: FrozenContractRecord cũ (pre-CP-67, không có `signature_hash`) → marshal/unmarshal byte-identical, gate bỏ qua check.
- [ ] AC-8: 4 test signatures green:
  - `TestExtractCanonicalSignaturesGo`
  - `TestCanonicalSignatureHashDeterministic`
  - `TestRuleSignatureLockFailsOnSignatureModification`
  - `TestRuleSignatureLockPassesWhenOnlyBodyModified`

## 7. Out of Scope

- LSP documentSymbol integration cho Kotlin/TS (follow-up sau CP-67).
- Tool face schemas (P-1 / Task-378).
- Prompt templates (P-3 / Task-380, P-4 / Task-381).
- Flow topology wiring (P-5 / Task-382).

## 8. Completion Notes

- result: `todo`
- follow-ups: consumed by Task-380/381/382.
- upstream docs updated: `todo`
