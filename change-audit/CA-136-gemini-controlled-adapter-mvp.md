# CA-136: Gemini Controlled Adapter MVP

## Scope

Add the first controlled-mode Gemini provider adapter over ACP while keeping unproven capabilities disabled.

## Completed

- Added `apps/local-runner/internal/runner/gemini_adapter.go`, implementing `ProviderRuntimeAdapter` for Gemini over `gemini --acp`.
- The MVP adapter initializes ACP, creates a Gemini session, sends `session/prompt`, emits normalized `message_delta`, `message_completed`, and `turn_completed` events, and surfaces ACP errors as Go errors for the shared finalizer.
- Forced `--approval-mode plan` so Gemini does not get uncontrolled write/tool authority before runner-owned approvals are wired.
- Wired live `ProviderRegistryFor` to register Gemini as available with only conservative capabilities: `Streaming`, `SkillSelection`, and `Interrupt`.
- Kept `DefaultProviderRegistry` placeholder-safe for Gemini.
- Added `apps/local-runner/internal/runner/gemini_adapter_test.go` for adapter args, streaming, final-result fallback, ACP error propagation, prompt preparation, env wiring, and registry capability gating.
- Marked Task-165 complete and updated CP-40/downstream task links.

## Verification

- `go test ./internal/runner -run 'TestGemini(Adapter|ACP|Registry)|TestStartSessionGeminiACP|TestSendMessageGeminiACP|TestJsonRpcErrorMessageHandlesGeminiACPShapes|TestProviderKeyFromModel' -count=1` from `apps/local-runner`
- `go test ./internal/runner -count=1` from `apps/local-runner`
- `go test ./... -count=1` from `apps/local-runner`

## Residual Notes

- Gemini resume is intentionally not advertised yet.
- Approval events, MCP tools, `ask_user`, `spawn_agent`, file events, and vision remain disabled until Task-166 proves the necessary structured protocol behavior.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-165
change_type: feature
summary: Add conservative Gemini controlled ACP adapter MVP
# --->8---
