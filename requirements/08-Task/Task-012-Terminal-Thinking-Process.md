# Task 012: Terminal-Style Provider Progress Streaming

## Problem

Workflow steps call AI provider CLIs through the local runner, but the UI previously showed only a loading state until the provider command finished. This made long-running steps feel stalled compared with official CLI tools, where users can see terminal output and intermediate progress in real time.

## Goal

Show live provider process output in the workflow run detail page while each workflow step is running.

The goal is not to expose hidden model reasoning. The UI should stream observable CLI stdout/stderr and provider protocol progress events only.

## User Experience

- When a workflow step is running, the Run Logs panel should receive new `provider_stream` rows during execution.
- Streamed rows should show the output text directly, with a badge for `stdout`, `stderr`, or the provider stream name.
- Existing final output, artifact creation, session reuse, approval gates, and error handling should keep working.
- If streaming is unavailable, the system should still return the final `sendMessage` result through the existing non-streaming path.

## Implemented Design

### Local Runner

Added a streaming session message endpoint:

```text
POST /sessions/message/stream
Content-Type: application/x-ndjson
```

The endpoint returns newline-delimited JSON events:

```json
{"type":"chunk","stream":"stdout","message":"..."}
{"type":"chunk","stream":"stderr","message":"..."}
{"type":"result","result":{...PromptExecutionResult}}
{"type":"error","code":"session_dead","error":"...","details":"..."}
```

The existing `POST /sessions/message` endpoint remains unchanged for compatibility.

### Provider Streaming Sources

- `gemini_acp`: streams `session/update` `agent_message_chunk` text as stdout chunks.
- `claude_stream_json`: reads stdout/stderr pipes while the `claude` print command is still running, forwards parsed text where possible, and preserves the final result parsing.
- `codex_mcp`: forwards JSON-RPC notifications received before the final tool response as stdout chunks.

### Admin Web Gateway

`LocalRunnerGateway.sendMessage` now accepts an optional stream callback:

```ts
sendMessage(request, {
  onStream: async (event) => {
    // event.type === "chunk"
  },
});
```

When a callback is provided, the HTTP gateway calls `/sessions/message/stream`, parses NDJSON incrementally, forwards chunk events, and returns the final `PromptExecutionResult` from the terminal `result` event.

### Workflow Runtime Logs

Workflow execution uses the streaming `sendMessage` option and writes buffered stream chunks into `workflow_run_logs`:

```text
provider_stream:{"stream":"stdout","text":"..."}
```

Chunks are batched before log insertion to avoid writing one database row per token-sized event.

### Run Detail UI

The existing Run Logs panel recognizes `provider_stream:` messages and renders:

- label: `provider_stream`
- badge: stream name such as `stdout` or `stderr`
- body: streamed provider text

## Files Changed

- `apps/local-runner/internal/cli/root.go`
- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/types.go`
- `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`
- `apps/admin-web/src/domain/gateway/local-runner-gateway.ts`
- `apps/admin-web/src/domain/model/entity/local-runner.ts`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`

## Verification

Completed:

```bash
GOCACHE=/private/tmp/flowpilot-go-cache go test ./internal/runner ./internal/cli -run '^$'
npm test -- src/features/workflow-engine/workflow-run-log-fallback.test.ts src/domain/usecase/workflow-engine/workflow-engine-usecases.test.ts
git diff --check
```

Known environment limitations:

- Full Go tests fail on this macOS environment because existing tests invoke `powershell`, which is not available on PATH.
- Full `tsc --noEmit` currently reports unrelated pre-existing project-wide type errors.
- GitNexus CLI in this environment could read repo status but could not run `impact` or `detect_changes`; `detect_changes` is not available in the installed CLI build.

## Acceptance Criteria

- A running workflow step inserts live `provider_stream` logs before the final step result is available.
- The final `PromptExecutionResult` still drives artifact creation and step completion.
- Existing session recovery behavior still handles `session_dead`, `session_terminated`, and thread-missing fallback.
- The Run Logs UI displays streamed provider output as readable terminal-style rows.
