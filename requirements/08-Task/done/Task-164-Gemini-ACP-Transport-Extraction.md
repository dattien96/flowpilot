# Task-164: Gemini ACP Transport Extraction

## Metadata

- Document ID: `Task-164`
- Title: `Gemini ACP Transport Extraction`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-27`
- Last Updated: `2026-06-27`
- Parent Documents: [CP-40: Gemini Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-40-Gemini-Adapter-Plan.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [07 - Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md), [04-07 - Phase 7: Providers Capability Packaging](../../10-Refactor/New-System/04-07-Phase7-Providers-Capability-Packaging.md)
- Replaces: `None`
- Tags: `gemini, acp, transport, runner, ai-providers`

## AI Quick View

### Summary

- Extract Gemini ACP JSON-RPC request builders, text extraction, and response parsing from `sessions.go` into focused transport helpers.
- Preserve the legacy `gemini_acp` session path by making it call the extracted helpers without changing behavior.
- Add fake-wire tests before any live registry enablement.

### Current Ask

- This task is complete: Gemini ACP request/response helpers are extracted and covered by focused tests, and the legacy session path still passes.

### Key Decisions

- `T-1` This task does not expose Gemini as an available controlled provider.
- `T-2` Extracted helpers must stay protocol-shaped and testable without a real Gemini CLI.
- `T-3` Capability flags remain false until later tasks prove controlled behavior.

### Constraints

- Do not change Codex or Claude behavior.
- Do not add a Gemini-only desktop endpoint or bypass the provider-neutral runner path.
- Keep legacy `sessions.go` Gemini ACP prompt behavior equivalent.

### Open Questions

- Does live ACP expose approval/tool/question events? Deferred to Task-166.
- Is ACP resume safe across Gemini homes? Deferred to Task-167.

### Source Refs

- `CP-40` sections `P-1`, `P-2`, `P-5`, `G-01`, `G-16`.
- Code: `apps/local-runner/internal/runner/sessions.go`, `provider_registry.go`, `provider_event.go`.

## 1. Goal

Create a reusable, tested Gemini ACP transport layer that can support a controlled adapter without duplicating legacy session code.

## 2. Parent Links

- coding plan: `CP-40`
- tech design: `SD-12`, `SD-06`
- system spec: provider-neutral workflow/session behavior from `SS-11`
- specific upstream ids: `CP-40 P-1`, `P-2`, `P-5`, `G-01`, `G-16`

## 3. Trigger

CP-40 requires a Gemini controlled adapter, but the only current ACP implementation is embedded in `sessions.go`. The helper extraction must land first so the adapter can be built against a small, testable transport boundary.

## 4. Exact Change

- `T-1` Add Gemini ACP helper file(s) for initialize params, `session/new`, `session/prompt`, response text extraction, session id extraction, and JSON-RPC error normalization.
- `T-2` Update the legacy `sessions.go` Gemini ACP branch to call the helper functions.
- `T-3` Add unit tests for Gemini ACP payload shape, streamed text extraction, result text fallback, session id extraction, and JSON-RPC error messages.
- `T-4` Add malformed/unsupported payload tests to prove helpers fail empty rather than panic.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/sessions.go`, new `apps/local-runner/internal/runner/gemini_acp_transport.go`, new transport tests
- modules: local runner provider/session transport
- routes: none
- tables: none

## 6. Acceptance Check

- Gemini ACP helper tests pass without a real Gemini CLI.
- Existing session tests still pass.
- `sessions.go` no longer owns Gemini ACP helper logic directly.
- No provider registry status or capability flag changes are made in this task.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Extracted Gemini ACP request builders match the legacy payload shapes.
- [x] `DOD-2` Streamed `agent_message_chunk` text and final result text are parsed by helper tests.
- [x] `DOD-3` JSON-RPC error extraction and missing-session-id handling are covered.
- [x] `DOD-4` Legacy `gemini_acp` session path compiles and calls the extracted helpers.
- [x] `DOD-5` Targeted runner tests pass with no Codex/Claude regression.

## 7. Out of Scope

- Live Gemini adapter registration.
- Approval, `ask_user`, `spawn_agent`, file events, and MCP tool registration.
- Cross-account resume and transcript extraction.
- Desktop UI changes.

## 8. Completion Notes

- result: done
- implementation notes: added `gemini_acp_transport.go` for ACP request builders, streamed text extraction, final result fallback parsing, and session id extraction; `sessions.go` now calls those helpers for Gemini ACP session creation and message sending.
- verification: `go test ./internal/runner -run 'TestGeminiACP|TestStartSessionGeminiACP|TestSendMessageGeminiACP|TestJsonRpcErrorMessageHandlesGeminiACPShapes' -count=1`; `go test ./internal/runner -count=1`; `go test ./... -count=1` from `apps/local-runner`.
- follow-ups: Task-165, Task-166, Task-167
- upstream docs updated: `CP-40` Child Documents now links the task chain.
