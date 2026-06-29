# CA-137: Gemini ACP Tool And Permission Protocol

## Scope

Add conservative Gemini ACP tool-event and permission-request support without advertising full approval/MCP parity before authenticated live validation.

## Completed

- Added `apps/local-runner/internal/runner/gemini_event_mapper.go` to map ACP `tool_call` and `tool_call_update` session updates into normalized provider tool events.
- Added Gemini ACP `session/request_permission` handling in `geminiAdapter`, routing through `TurnBridge.RequestApproval` and replying with ACP `selected` or `cancelled` permission outcomes.
- Added `writeJsonRpcResponse` for provider-to-client JSON-RPC replies.
- Added FlowPilot MCP server injection to Gemini `session/new` using the existing runner-hosted MCP bridge token.
- Added tests for tool-event mapping, ACP permission details/response selection, permission denial replies, and MCP server payload injection.
- Kept Gemini capability flags conservative: approval, MCP, file events, resume, and vision remain false until live validation proves end-to-end control.

## Verification

- `go test ./internal/runner -run 'TestGemini|TestStartSessionGeminiACP|TestSendMessageGeminiACP|TestJsonRpcErrorMessageHandlesGeminiACPShapes|TestProviderKeyFromModel' -count=1` from `apps/local-runner`
- `go test ./internal/runner -count=1` from `apps/local-runner`
- `go test ./... -count=1` from `apps/local-runner`

## Residual Notes

- Live Gemini ACP initialize was probed on local Gemini CLI `0.40.1`; it advertised `loadSession`, image/audio/embedded-context prompt capability, and HTTP/SSE MCP capability.
- The same probe could not create a session because local Gemini auth/API key is missing, so full live approval/MCP/agent parity remains unclaimed and deferred to Task-167.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-166
change_type: feature
summary: Add conservative Gemini ACP tool and permission protocol support
# --->8---
