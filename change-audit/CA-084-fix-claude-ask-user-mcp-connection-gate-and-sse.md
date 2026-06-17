# CA-084 — Fix Claude ask_user Live Flow: MCP Connection Gate + GET SSE (Task-055)

## Scope

Runner — Claude provider adapter + runner-hosted permission/ask_user MCP server:

- `apps/local-runner/internal/runner/claude_permission_mcp.go`
- `apps/local-runner/internal/runner/claude_mcp_server.go`
- `apps/local-runner/internal/runner/claude_adapter.go`
- tests: `claude_permission_mcp_test.go`, `claude_mcp_server_test.go`, `claude_mcp_matrix_test.go`, `claude_e2e_test.go` (new)

Follows on from CA-083 (made the per-turn `--mcp-config` available in both YOLO states). CA-083
fixed tool *availability in the config*; the live `ask_user` call was still broken by a deeper
MCP connection-handshake defect, fixed here. Closes the live-model gap left open in DOD `T-29`/`PP-31`.

## Completed

The structured-question `ask_user` MCP tool never reached the Claude model in the running desktop
app — the QuestionCard options UI never appeared and the model asked in plain text instead. Four
compounding causes, root-caused by reproducing the failure against a real `claude` 2.1.179 process
and inspecting its MCP debug log.

### Defect A — built-in `AskUserQuestion` shadowed the MCP tool

Claude has a native `AskUserQuestion` tool. Asked to "ask the user", the model called the built-in
tool instead of `mcp__flowpilot__ask_user`. The built-in runs inside the headless CLI with no TTY:
it returns "the user did not answer the questions" immediately and renders the question as plain
text, never reaching FlowPilot's bridge → no `user_question_required` event → no QuestionCard.

**Fix:** `claudeArgs` adds `--disallowed-tools AskUserQuestion` in all YOLO states, forcing the
model onto the MCP tool. The built-in is useless headless anyway, so this is strictly better.

### Defect B — `ask_user` advertised an empty input schema

`claudeMCPToolDefs` declared `ask_user` with `inputSchema: {"type":"object"}` (no properties), so
the model had no signal how to call it and preferred the well-defined built-in.

**Fix:** `ask_user` now advertises `prompt`/`options[]`/`multiSelect` with `required: [prompt]`,
mirroring the Codex registration (`codexAskUserMcpServer`).

### Defect C (primary) — server declined the GET SSE probe with 405

`claudeMCPServer.ServeHTTP` returned `405` for `GET` ("we only do request/response, so decline
GET"). But Claude's Streamable-HTTP client opens a `GET` SSE stream during the handshake and only
marks the server **connected** once it succeeds. With 405 the server stayed `status: "pending"` and
**its tools were never exposed to the model** — even though `initialize` + `tools/list` succeeded
over POST. (The `approve` permission tool still worked because Claude invokes the
`--permission-prompt-tool` via a separate internal path, not the model's tool list — which is why
the first manual test showed an approval card but no usable `ask_user`.)

**Fix:** `GET` now returns an open `text/event-stream` (`serveSSE`): `200`, a `: connected`
prelude, then keepalive comments until the request context is cancelled (claude disconnects). One
goroutine per live turn. FlowPilot never pushes server→client messages, so the stream carries only
keepalives. Verified: with 405 the model gets no tool even when the prompt is gated; with SSE the
server reports `connected` and the model calls `ask_user`.

### Defect D — prompt delivered before the async MCP connection completed

Claude connects `--mcp-config` servers asynchronously ("[MCP] --mcp-config servers running fully
async (nonblocking)" in its debug log). `SendTurn` wrote the user prompt immediately after spawn,
so the **first** turn's tool set was snapshotted before the ~0.1–1.2s connect finished and never
refreshed mid-turn. Because the process model is **spawn-per-turn**, every turn is a "first turn"
→ `ask_user` was effectively never available. (Reproduced: a two-turn session has the tool on turn
2, not turn 1; a single turn whose prompt is withheld until `tools/list` arrives has it.)

**Fix:** runner-side connection gate. `claudeMCPServer` tracks a per-token ready channel
(`register` creates it, `signalReady` closes it on the first `tools/list`, `unregister` deletes it);
`waitReady(ctx, token, timeout)` blocks until the connection is live, the timeout elapses, or ctx
is cancelled. `SendTurn` calls `waitReady` before `writeUserTurn` when a per-turn MCP config was
written (`mcpToken != ""`), bounded by the adapter's `mcpReadyTimeout`
(`0` → `claudeMCPReadyDefaultTimeout = 10s`). A slow/failed connect degrades to sending anyway
rather than hanging.

### Desired UX confirmed

With YOLO=on, all other tool calls auto-pass (bypassPermissions, no approval card), but `ask_user`
still surfaces the confirm/options card — exactly the intended human-in-the-loop behavior. This is
the YOLO=on + `--mcp-config` path established in CA-083, now actually reaching the model.

## Verification

- `go build ./...` clean in `apps/local-runner`.
- New unit tests: `TestClaudeArgsDisablesBuiltinAskUserQuestion` (both YOLO states),
  `TestClaudeAskUserToolAdvertisesSchema`, `TestClaudeMCPServerPromptGate` (unblock-on-tools/list,
  timeout, ctx-cancel, unknown-token), `TestClaudeMCPGetOpensSSEStream`.
- Real-`claude` end-to-end: `TestClaudeAskUserEndToEnd` (real adapter + real runner-hosted Go MCP
  server via `httptest` + real `claude` CLI) — the model calls `mcp__flowpilot__ask_user`. Skipped
  by default; run with `FLOWPILOT_CLAUDE_E2E=1` (PASS in ~8s).
- No regression: `TestClaudeSendTurnMcpAvailabilityMatrix` (BUG-073, YOLO×MCP),
  `TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade`, `TestClaudeArgsYoloPosture` all pass. The
  matrix test gets a short `mcpReadyTimeout` (its scripted fake process never connects to the MCP
  server, so the gate would otherwise wait the full default).
- Whole-package: `go test ./internal/runner` shows the **identical 23 pre-existing failures** on the
  clean tree and the changed tree (verified by `git stash` + re-run, ×2 each — stable). All 23 are
  environmental (Google Drive OAuth/credentials/launcher absent, skills-source env). No new
  regressions.
- Live desktop app: user-confirmed after restarting `just dev` (the runner launches via `go run`,
  recompiling the fix) — the QuestionCard options card renders for Claude.

## Residual Notes

- **Codex `ask_user` is still broken** and out of scope. Codex registers `ask_user` inline on
  `thread/start` (`codexAskUserMcpServer`, shape `{name, tools:[]}`), has no runner-hosted HTTP MCP,
  and `handleInbound` only recognizes approval/elicitation methods — there is **no `tools/call`
  handler** to bridge a Codex ask_user call to `bridge.AskQuestion`. The SSE/connection-gate fixes
  here are Claude-specific. Fixing Codex needs a spike to capture the codex-cli app-server's real
  inbound tool-call frame, then a registration-shape fix + inbound handler.
- **The connection gate adds up to `mcpReadyTimeout` of first-turn latency only on a failed/slow
  connect**; a healthy local connect resolves in ~0.1–1.2s. The timeout is a degrade-not-hang
  backstop, not the expected path.
- **Spawn-per-turn is unchanged** (07 plan "Final Decision"): each prompt spawns a fresh `claude`
  and reconnects MCP, so the gate runs every turn. This is inherent to the existing model, not
  introduced here. The CA-083 future-improvement note (host google-drive as a persistent
  runner-hosted HTTP MCP) would also let MCP connect once; not done here.
- The working-tree change to `requirements/10-Refactor/New-System/08-Desktop-Chat-New-Plan.md` was
  NOT produced by this work and is left untouched for separate review.
- GitNexus interactive tools (`gitnexus_impact`/`context`/`query`) were not connected this thread;
  edits relied on manual call-graph inspection (`SendTurn` → `claudeArgs` /
  `writeClaudeMCPConfig` → `claudeMCPServer.dispatch` / `serveSSE` / `waitReady`).
