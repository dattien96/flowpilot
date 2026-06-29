# CA-138: Gemini ACP Resume Scaffolding

## Scope

Add deterministic Gemini provider-session capture and ACP resume request support while keeping live resume parity unclaimed until authenticated Gemini validation is available.

## Completed

- Added `apps/local-runner/internal/runner/gemini_session.go` to share FlowPilot synthetic session id to Gemini ACP session id mappings across per-turn adapter instances.
- Added Gemini ACP `session/load` request params in `gemini_acp_transport.go`.
- Updated `geminiAdapter` to:
  - choose `session/load` when a real Gemini provider session id is known
  - record Gemini ACP session ids after `session/new` or `session/load`
  - persist Gemini provider session metadata through `ProviderSessionStore`
- Wired the live runner registry to provide the shared Gemini session map and provider-session store.
- Updated interactive resume-handle refresh so completed Gemini turns populate `realProviderSessionID`.
- Seeded restored Gemini runs with their persisted real provider session id before the next turn.
- Added tests for ACP `session/load`, prompt session-id reuse, provider-session upsert, and interactive resume-handle capture.
- Follow-up review fixes tightened Gemini shared-session resume by using `session/load` for explicit real resume ids, scoping synthetic-to-real mappings by Gemini account scope, and rejecting unmapped synthetic legacy ids.
- Second review-loop fixes capture prompt-result Gemini session ids, reject unmapped synthetic resume ids in both adapter and legacy session paths, and avoid persisting non-UUID step keys as provider-session FK values.
- Follow-up requirements review expanded `CP-40` and `Task-167` to track post-Codex/Claude Gemini parity gaps: usage/quota errors, account metadata, external MCP, child-agent lifecycle, Drive sync/restore, token/context reporting, model metadata, history replay, and attachment fallback.
- Phase-document review fixed upstream traceability so `CP-40` links its SS parents and `Task-167` links both the `P-*` work slices and `G-10` through `G-27` validation scope.
- Persisted Gemini restart resume now fails explicitly with `resume_unsupported` instead of falling through Codex/Claude-only recovery paths, while completed in-memory Gemini chats remain reopenable read-only.
- Added review-driven tests for mapped and unmapped synthetic Gemini resume ids, mapping updates when `session/load` returns a new provider session id, Gemini-specific persisted resume failure, and in-memory completed Gemini reopen behavior.

## Verification

- `go test ./internal/runner -run 'TestGemini|TestStartSessionGeminiACP|TestSendMessageGeminiACP|TestJsonRpcErrorMessageHandlesGeminiACPShapes|TestProviderKeyFromModel' -count=1` from `apps/local-runner`
- `go test ./internal/runner -count=1` from `apps/local-runner`
- `go test ./internal/runner -run 'TestGeminiAdapter|TestStartSessionGemini|TestDetermineProviderSessionID|TestResumeRunGeminiPersistedResumeIsExplicitlyUnsupported|TestResumeRunInMemoryCompletedGeminiRemainsReadable|TestRefreshResumeHandleCapturesGeminiRealSession' -count=1` from `apps/local-runner`
- `go test ./...` from `apps/local-runner`

## Residual Notes

- Gemini `Resume` capability remains false because local live `session/new` is blocked by missing Gemini auth/API key, so cross-process/session artifact portability is not yet proven.
- Updated live probe after user login attempt: current Codex HOME still reaches Gemini ACP initialization but `session/new` fails with `Gemini API key is missing or not configured`; `HOME=/Users/tiendat` reaches OAuth/config files but `session/new` fails with `UNSUPPORTED_CLIENT` because this client is no longer supported for Gemini Code Assist for individuals.
- Gemini-as-source handoff remains disabled until authenticated live session artifacts can be inspected and a transcript extractor is added.
- Full CP-40 live DOD remains in `Task-167` until a supported Gemini CLI auth method is configured for the runner process.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-167
change_type: feature
summary: Add Gemini ACP resume session capture and reviewed session-load safeguards
# --->8---
