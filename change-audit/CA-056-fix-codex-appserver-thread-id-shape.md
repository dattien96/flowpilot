# CA-056: Fix Codex AppServer Thread ID Shape

## Summary

- Fixed the live Codex app-server adapter to accept the generated `thread/start` response shape from `codex-cli 0.138.0` (`result.thread.id`).
- Updated `turn/start` request shaping to send a text `UserInput[]` while preserving existing fake-server compatibility.
- Added generated app-server notification mappings for agent message deltas, completed items, and turn completion.
- Started live Codex app-server processes with the active FlowPilot Codex account
  (`CODEX_HOME`) and added a process-env fallback for local dev.
- Mapped Codex usage-limit/system errors to `turn_failed` instead of empty successful completion.
- Added regression tests for nested thread/turn response IDs and generated notification names.

## Changed Files

- `apps/local-runner/internal/runner/codex_adapter.go`
- `apps/local-runner/internal/runner/codex_appserver.go`
- `apps/local-runner/internal/runner/codex_appserver_process.go`
- `apps/local-runner/internal/runner/codex_appserver_test.go`
- `apps/local-runner/internal/runner/codex_event_mapper.go`
- `apps/local-runner/internal/runner/codex_event_mapper_test.go`
- `apps/local-runner/internal/runner/provider_registry.go`
- `requirements/09-BugFix/done/BUG-045-Codex-AppServer-Thread-Start-No-ThreadId.md`

## Verification

- `go test ./internal/runner -run 'Test(CodexAdapterTurnStreams|CodexAdapterAcceptsGeneratedAppServerThreadAndTurnShapes|CodexAdapterApprovalRoundTrip|MapCodexNotification|SendTurnWithRetry)'`
- `codex --version` reported `codex-cli 0.138.0`.
- Live `codex app-server --listen stdio://` smoke confirmed `thread/start` returns `result.thread.id`.
- Live `codex app-server --listen stdio://` smoke confirmed the runner's `thread/start` params are accepted.
- Temporary live runner on port `4318` streamed `fallback-account-ok` through the
  HTTP/SSE runner API.

## Residual Risk

- Full `go test ./internal/runner` is blocked by unrelated local environment failures in Google Drive MCP and `powershell`-based provider tests.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-045
change_type: fix
summary: Fix Codex AppServer Thread ID Shape
# --->8---
