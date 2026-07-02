# CA-135: Gemini ACP Transport Extraction

## Scope

Extract Gemini ACP transport helpers from the legacy session runtime so the upcoming controlled Gemini adapter can reuse a focused, tested protocol boundary.

## Completed

- Added `apps/local-runner/internal/runner/gemini_acp_transport.go` for Gemini ACP initialize, `session/new`, `session/prompt`, streamed text, result text, and session id helpers.
- Updated `apps/local-runner/internal/runner/sessions.go` so the existing `gemini_acp` session creation and message-send paths call the extracted helpers.
- Added `apps/local-runner/internal/runner/gemini_acp_transport_test.go` covering request payloads, streamed `agent_message_chunk` extraction, result text fallback, session id extraction, JSON-RPC error shapes, and malformed payload handling.
- Split CP-40 into Task-164 through Task-167 and marked Task-164 complete.

## Verification

- `go test ./internal/runner -run 'TestGeminiACP|TestStartSessionGeminiACP|TestSendMessageGeminiACP|TestJsonRpcErrorMessageHandlesGeminiACPShapes' -count=1` from `apps/local-runner`
- `go test ./internal/runner -count=1` from `apps/local-runner`
- `go test ./... -count=1` from `apps/local-runner`

## Residual Notes

- Gemini remains a placeholder in the provider registry. Live controlled adapter registration is deferred to Task-165.
- Approval, MCP tool, `ask_user`, `spawn_agent`, resume, and handoff parity are not claimed by this task.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-164
change_type: refactor
summary: Extract and test Gemini ACP transport helpers
# --->8---
