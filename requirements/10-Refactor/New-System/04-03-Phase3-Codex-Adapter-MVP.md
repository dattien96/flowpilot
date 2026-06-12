# 04-03 — Phase 3: Codex Adapter MVP

> Part of the `04` coding plan. Code-grounded against the Go runner
> (`apps/local-runner`, package `internal/runner`). Replaces the Phase 2 fake
> adapter with the real Codex app-server adapter behind the same contract.

## Goal

Implement Codex `app-server` as the first real provider adapter: shared process,
async JSON-RPC dispatcher, thread + turn lifecycle, streaming, and event mapping
to normalized `ProviderEvent`s. (Approval + YOLO + finalizer are Phase 4.)

## Adapter boundary

Codex JSON-RPC and event names stay inside the Codex code. FlowPilot core sees only
the shared contract (`04`). The desktop client never speaks raw app-server JSON-RPC.

## Package layout (new Go files)

```text
apps/local-runner/internal/runner/
  provider_event.go          # normalized ProviderEvent types + constants
  codex_appserver.go         # shared process mgmt + Codex JSON-RPC methods + dispatcher
  codex_event_mapper.go      # Codex notification -> ProviderEvent
```

Reuse (framing only): `commandContextFn` (`sessions.go:22`), `writeJsonRpcRequest`
(`sessions.go:120`), `readJsonRpcMessage` (`sessions.go:139`), process-spawn + pipe
wiring from `StartSession` (`sessions.go:448`), `SessionStreamCallback`
(`sessions.go:653`).

**Do NOT reuse** `readJsonRpcResponseWithHandler` (`sessions.go:157`) or the
`SendMu`/hardcoded-id-`3` model — they are synchronous/single-in-flight and cannot
multiplex a shared app-server.

## Shared app-server process

- Extend `Runner` (`runner.go:121`): `codexAppServer *codexAppServerHandle` (mutex).
  `Runner.workspace` (`runner.go:122`) is demoted to a default cwd; per-thread `cwd`
  is authoritative.
- `ensureCodexAppServer(ctx, scopeKey)`: start one `codex app-server --listen
  stdio://` if not running for that scope; reuse otherwise. (Account-scoped recreate
  is Phase 6; here assume the active account.)

## Codex JSON-RPC methods + async dispatcher

- Payload builders next to `geminiACP*Params` (`sessions.go:265`):
  `codexInitializeParams`, `codexThreadStartParams(cwd, sandbox, approvalMode)`,
  `codexThreadResumeParams`, `codexThreadListParams(cwd)`, `codexThreadReadParams`,
  `codexTurnStartParams(threadId, prompt, skill)`, `codexInterruptParams`.
- **Async dispatcher (replaces the synchronous helper):** one dedicated read-loop
  goroutine reads every line and dispatches —
  - responses → matched by **unique monotonic id** via `map[id]chan response`;
  - notifications → routed by **thread id** to the owning turn's event channel.
  Read loop **never blocks**; per-turn consumers drain their channel. Guard `stdin`
  writes with a mutex (many turns interleave on one pipe).

## Thread mapping + persistence

- Persist `(workspace, account, threadId, providerThreadId, model, status)`
  (`provider_session_store.go`).
- `thread/start` (cwd), `thread/resume`, `thread/list` (by cwd), `thread/read`.
  Codex owns the JSONL log; FlowPilot keeps only the mapping.

## Event mapping (`codex_event_mapper.go`)

- turn started → `turn_started`; text delta → `message_delta`; message complete →
  `message_completed`; tool/MCP call → `tool_started`/`tool_completed`; **command
  execution → `tool_started`/`tool_completed` with exit status (distinct event)**;
  file change → `file_changed`; turn error → `turn_failed`; complete →
  `turn_completed`. Emit through `SessionStreamCallback`.

## Runner-side prompt assembly

Before `turn/start`, the runner injects `injectSkillContent` (`runner.go:1012`) +
`preparePromptForRequiredMcps` (`mcp_prompt_instructions.go:221`). (Base-prompt
build from step definitions is ported in Phase 5.)

## Milestones / acceptance

1. process starts, `initialize` succeeds.
2. `thread/start` + simple `turn/start` returns `turn_completed`.
3. streaming `message_delta`s render in the (real-wired) client.
4. `file_changed` and command-execution events map correctly.
5. `thread/list` by cwd; `thread/resume` reattaches.
6. two threads with different cwd run concurrently in one app-server.

## Tests (Go, mock `commandContextFn` per `sessions_test.go:89-129`)

- T-01 start/init, T-05 stream deltas, T-07 final-message separation.
- T-26 command-exec distinct mapping.
- T-13 `thread/list` by cwd; T-23 two concurrent threads.

## Definition of Done (checklist)

- [ ] `provider_event.go`, `codex_appserver.go`, `codex_event_mapper.go` added.
- [ ] Shared app-server starts; `initialize` succeeds (T-01).
- [ ] Async dispatcher: id→waiter map, per-thread routing, non-blocking read loop, stdin write mutex (no reuse of the synchronous helper / `SendMu` / id `3`).
- [ ] `thread/start|resume|list|read` + `turn/start` wired; `(workspace, account, threadId)` persisted.
- [ ] Streaming `message_delta` (T-05); final answer separated (T-07).
- [ ] Event mapping incl. distinct command-execution events (T-26) and `file_changed`.
- [ ] Runner-side skill + MCP injection before `turn/start`.
- [ ] Two concurrent threads in one app-server (T-23); `thread/list` by cwd (T-13).
- [ ] **Review gate:** human + AI review this checklist after the phase.
