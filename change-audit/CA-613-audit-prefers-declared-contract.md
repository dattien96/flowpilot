# CA-613: Audit prefers declared contract over stale grok package (run-127174)

## What

run-127174 Rag Harness ClampChecked: `validate` passed (exit 0) nhưng `audit` vẫn park `blocked_missing_feature_key` với `feature_key=grok` rồi escalate `WAITING_USER_APPROVAL`. Ảnh chụp F2 audit WAITING + Thinking 3m+. Thực tế contract đã declare `calc-format` từ implement, `FEATURE-KEYS.md` có `calc-format`.

## Why

- `context.produce` (08:58:04) resolve `grok` từ catalog `.grok/skills/**` noise (discussion/history chứa "hello grok"), `ConfidenceVerified`.
- `runAuditNode` (`flow_validate_audit_dispatch.go:943`) chỉ đọc `pkg.FeatureKey` từ `loadPlanContextPackage`, không nhìn contract. `BuildAuditDraft` thấy `grok` không có trong `FEATURE-KEYS.md` → `blocked_missing_feature_key` dù validation đã `passed` và contract `calc-format` đã `declared`.
- Đây là false park, không phải missing key thật. YOLO ON không auto-continue escalate (đúng thiết kế).

## Fix

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:939` trước `BuildAuditDraft`, nếu workspace có contract `ConfidenceDeclared` và `featureKeyRegistered(workspace, c.FeatureKey)` thì ghi đè `pkg.FeatureKey = c.FeatureKey`, `pkg.FeatureConfidence = ConfidenceVerified`.
- Chỉ ghi đè khi contract đã `declared` và key đã registered trong `FEATURE-KEYS.md`; không tự thêm key catalog-skill (`grok`, `flowpilot`, `docs`) vào registry. Không bypass `blocked_validation_failed`.
- Provider-agnostic: `grep -R ProviderKey flow_validate_audit_dispatch.go flow_audit_draft.go` 0 hit; `BuildAuditDraft` không nhánh provider. Claude/Codex/Grok cùng path.

Will not undo: CA-587 WAITING stamp, CA-585 RUNNING trước exec, CA-610/611/612 paste, BUG-231 Continue/Stop.

## Tests

- New `run127174_audit_prefers_declared_key_test.go` (additive, no old edit):
  - `TestRun127174_AuditPrefersDeclaredContractOverGrokPackage` — pkg grok + contract calc-format declared → override → draft `ready`
  - `TestRun127174_AuditNoContractStillBlockedOnGrok` — không contract → vẫn `blocked_missing_feature_key`
  - `TestRun127174_AuditUnregisteredContractKeyStillBlocked` — contract key không registered → không override, vẫn block
  - `TestRun127174_AuditPassedValidationRequiredEvenWithContract` — validation `failed` → vẫn `blocked_validation_failed` dù có contract
- Old suite green: `go vet ./internal/runner`, `go test ./internal/runner -run TestRun127174|TestSetFlowStepAwaitingUser|TestBuildAuditDraft -count=1` pass.

## Provider parity

Agnostic — không nhánh ProviderKey. Chứng minh bằng grep 0 hit và 4 test trên single workspace; claude/codex/grok chia sẻ cùng override.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-56
change_type: bugfix
summary: audit prefers declared contract calc-format over stale grok package (run-127174)
# --->8---
