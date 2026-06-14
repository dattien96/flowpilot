# BUG-045 Codex AppServer Thread Start No ThreadId

## Symptom

Desktop turns using the live Codex app-server path failed after retrying:

```text
[recovering: re-sending turn after a recoverable error (attempt 2/3)]
[recovering: re-sending turn after a recoverable error (attempt 3/3)]
codex thread/start returned no threadId
```

After that protocol fix, live turns could still show only:

```text
Turn completed.
```

with no assistant response.

## Root Cause

The runner adapter expected the older assumed response shape:

```json
{ "threadId": "..." }
```

The installed `codex-cli 0.138.0` app-server returns the generated protocol shape:

```json
{ "thread": { "id": "..." } }
```

The same schema generation also showed related live protocol shapes:

- `turn/start` returns `{ "turn": { "id": "..." } }`
- `turn/start` expects `input` as `UserInput[]`, not a raw string
- live notifications use names such as `item/agentMessage/delta`, `item/completed`, and `turn/completed`

The no-response follow-up had a second cause: the live app-server launcher ignored
the active FlowPilot Codex account and started Codex with the process default
`~/.codex` account. That account was usage-limited. Codex emitted an `error`
notification and a failed `turn/completed`, but the mapper ignored the error and
projected the failed turn as an empty successful completion.

## Fix

- Accept both legacy top-level IDs and generated nested IDs for thread and turn responses.
- Encode `turn/start` input as a text `UserInput[]`.
- Map generated live app-server notification names into normalized provider events.
- Start the live Codex app-server with the active provider account's `CODEX_HOME`;
  fall back to the process `CODEX_HOME` when account rows are not available.
- Map Codex app-server `error` notifications and failed `turn/completed` payloads
  to normalized `turn_failed` events.
- Preserve backward-compatible fake-server protocol shapes used by existing tests.

## Verification

- `go test ./internal/runner -run 'Test(CodexAdapterTurnStreams|CodexAdapterAcceptsGeneratedAppServerThreadAndTurnShapes|CodexAdapterApprovalRoundTrip|MapCodexNotification|SendTurnWithRetry)'`
- Live app-server smoke against `codex-cli 0.138.0` verified `thread/start` returns `result.thread.id`.
- Live app-server smoke verified the runner's `thread/start` params are accepted.
- Temporary live runner on port `4318` streamed `message_delta`, `message_completed`,
  and `turn_completed` for a real Codex turn using the active/fallback account path.

## Open Notes

- Full `go test ./internal/runner` still has unrelated environment-sensitive failures around Google Drive MCP setup and `powershell` lookup.
