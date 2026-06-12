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
  stdio://` if not running for that scope; reuse otherwise. **`scopeKey` = the active
  provider account id** (Phase 6 adds account-scoped recreate; here assume the active
  account).
- **`initialize` captures the negotiated protocol version + capabilities** on the
  handle; the adapter **degrades gracefully** if a method is unsupported by the
  installed build (e.g. `thread/list` / `thread/read` / `thread/resume`) instead of
  hanging — see the 06 Part D build-verification caveat.

## Codex JSON-RPC methods + async dispatcher

- Payload builders next to `geminiACP*Params` (`sessions.go:265`):
  `codexInitializeParams`, `codexThreadStartParams(cwd, sandbox, approvalMode, mcpServers)`,
  `codexThreadResumeParams`, `codexThreadListParams(cwd)`, `codexThreadReadParams`,
  `codexTurnStartParams(threadId, prompt, skill)`, `codexInterruptParams`.
- **MCP servers on the thread.** `thread/start` carries an `mcpServers` list — ACP
  precedent: `geminiACPSessionNewParams` already passes `mcpServers`
  (`sessions.go:276`). Attach the **FlowPilot MCP proxy** here so required MCPs **and**
  the Phase-4 `ask_user` custom tool are reachable. Decide process-level vs
  per-`thread/start` wiring and document it; verify the param against the installed
  Codex build.
- **Async dispatcher (replaces the synchronous helper).** One dedicated read-loop
  goroutine reads every message and dispatches by **three** kinds:
  - **responses** (`id`, no `method`) → matched by unique monotonic id via
    `map[id]chan response`; a per-request `ctx`/timeout removes the waiter and fails
    the caller if no response arrives (no leaked waiters);
  - **notifications** (`method`, no `id`) → routed by **thread id** to the owning
    turn's event channel;
  - **inbound requests** (`method` **and** `id` — server→client, e.g. command/patch
    approval or elicitation) → routed to a request handler that **must reply** with a
    matching-id response. Approval/`ask_user` reply logic is Phase 4, but the
    dispatcher must already route this third kind — the current
    `readJsonRpcResponseWithHandler` (`sessions.go:157`) cannot, its handler only
    observes.
  The read loop **never blocks**: sends to per-turn channels are **non-blocking onto
  bounded buffers** (define overflow policy — spool or fail the turn, never block the
  loop). Guard `stdin` writes with a mutex (many turns interleave on one pipe).

## Process lifecycle & failure handling

- On read-loop EOF / read error / malformed JSON, or process exit: **fail every
  pending id-waiter and drain every per-turn channel with an error**, mark the handle
  dead, and let the next `ensureCodexAppServer` start a fresh process. A dead
  app-server must never leave a turn blocked — this is the exact hang the synchronous
  model caused.
- A `ctx` cancel on a turn issues `codexInterruptParams` and **drains trailing
  notifications** so the shared process is not left mid-turn (full interrupt/cancel
  semantics in `04-04`).

## Thread mapping + persistence

- Persist into `workflow_provider_sessions` (`04-02`): `working_directory`,
  `provider_account_id`, `provider_thread_id` (= `provider_session_id` for Codex),
  `model_name`, `status` — keep column names aligned with the P2 schema.
- `thread/start` (cwd), `thread/resume`, `thread/list` (by cwd), `thread/read`.
  Codex owns the JSONL log; FlowPilot keeps only the mapping.
- `providerTurnId` comes from the `turn/start` response (or the first `turn_started`);
  the mapper stamps it on every event of that turn. One-turn-per-thread (`04-02`'s
  `409`) is what makes thread-id routing unambiguous.

## Event mapping (`codex_event_mapper.go`)

- turn started → `turn_started`; text delta → `message_delta`; message complete →
  `message_completed`; tool/MCP call → `tool_started`/`tool_completed`; **command
  execution → `tool_started`/`tool_completed` with exit status (distinct event)**;
  file change → `file_changed`; turn error → `turn_failed`; complete →
  `turn_completed`. Emit through `SessionStreamCallback`.
- **Correlation:** the mapper fills the event base (run/step/session ids,
  `providerTurnId`) and **preserves provider event order within a turn**; the
  monotonic per-run **`seq` is assigned by runner core at a single serialization
  point** (not the adapter), so ordering stays stable across interleaved threads.

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
6. two threads with different cwd run concurrently in one app-server (verify the
   installed Codex build supports concurrent turns across threads on one stdio).

## Tests (Go, mock `commandContextFn` per `sessions_test.go:89-129`)

- T-01 start/init, T-05 stream deltas, T-07 final-message separation.
- T-26 command-exec distinct mapping.
- T-13 `thread/list` by cwd; T-23 two concurrent threads.
- T-37 inbound server→client request is routed to a reply-capable handler
  (Phase-4 readiness).
- T-38 dispatcher failure: a response timeout frees its waiter; process death drains
  all waiters/turns with error (no hung turns); per-turn backpressure never blocks
  the read loop.

## Definition of Done (checklist)

> Implemented in `apps/local-runner/internal/runner/` (`codex_appserver.go`,
> `codex_event_mapper.go`, `codex_adapter.go`) + tests (`codex_appserver_test.go`,
> `codex_event_mapper_test.go`) driven by a scripted fake app-server over in-memory
> pipes (no real `codex` binary here). `go vet` clean; all Codex tests pass.
> **Deferred to a real Codex build (06 Part D):** the live process spawn
> (`ensureCodexAppServer` + `initialize` capability capture), the runner-owned shared
> handle, and **flipping the registry from the fake adapter to `codexAdapter`** — the
> live system stays on the fake adapter until validated against the installed Codex.

- [x] `codex_appserver.go`, `codex_event_mapper.go`, `codex_adapter.go` added (`provider_event.go` from P2).
- [ ] Shared app-server **process** starts; `initialize` captures version/caps; unsupported methods degrade gracefully — _deferred: needs real `codex` (dispatcher is process-agnostic and tested over pipes)._
- [x] Async dispatcher handles **three** kinds — responses (id→waiter + ctx/timeout), notifications (per-thread routing), and **inbound server→client requests routed to a reply-capable handler** (T-37); non-blocking read loop with **bounded per-turn buffers**; stdin write mutex (does **not** reuse the synchronous helper / `SendMu` / id `3`).
- [x] **Process lifecycle:** read-loop EOF/error **drains all waiters + closes thread channels** (no hung turns, T-38); handle marked closed. _(Restart-on-next-ensure is part of the deferred ensure layer.)_
- [~] `thread/start` + `turn/start` wired and exercised; `resume/list/read` + `interrupt` param builders present; **`mcpServers` carried on `thread/start`** (proxy/required-MCP wiring is P4/P6); session persistence via `workflow_provider_sessions` is the InteractiveService path (codex adapter persistence wired on registry swap).
- [x] Streaming `message_delta` + final answer separated (`message_completed`/`turn_completed`) — adapter pump tested (`TestCodexAdapterTurnStreams`).
- [x] Event mapping incl. distinct command-execution events (exit-code→status) and `file_changed`; mapper leaves `providerTurnId` empty so **runner core stamps the canonical turn id + `seq`** (verified the adapter does not leak Codex's turn id).
- [ ] Runner-side skill + MCP injection before `turn/start` — _deferred: wired with the registry swap (uses `injectSkillContent` + `preparePromptForRequiredMcps`)._
- [~] Concurrency: dispatcher multiplexes concurrent calls + per-thread routing (tested); a full two-thread adapter run + `thread/list` by cwd verify against the installed build is deferred (T-13/T-23).
- [x] Dispatcher robustness tests: inbound-request routing (T-37); response timeout + process-death drain + backpressure (T-38).
- [ ] **Review gate:** human + AI review this checklist after the phase.
