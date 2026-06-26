# CA-085 — Fix Codex ask_user Live Flow: dynamicTools + item/tool/call + Nudge Tuning (Task-056)

## Scope

Runner — Codex provider adapter + app-server param builders, plus the shared ask_user reinforcement:

- `apps/local-runner/internal/runner/codex_appserver.go`
- `apps/local-runner/internal/runner/codex_adapter.go`
- `apps/local-runner/internal/runner/claude_adapter.go` (reinforcement parity only)
- tests: `codex_appserver_test.go`, `partd_test.go`, `codex_e2e_test.go` (new)

Sibling of CA-084 (Claude ask_user). Closes the Codex side of DOD `PP-31`/`T-29`.

## Completed

The structured-question `ask_user` tool never reached the Codex model in the running app — Codex
reported *"I can't access the interactive user prompt tool"* and answered in plain text. Three
causes, root-caused by dumping the real protocol (`codex app-server generate-json-schema
--experimental`) and reproducing against a real `codex app-server` (codex-cli 0.140.0).

### Defect A — wrong registration channel (inline mcpServers, silently ignored)

`ask_user` was registered as `mcpServers:[{name:"flowpilot", tools:[{name:"ask_user",...}]}]` on
`thread/start`. But the authoritative `ThreadStartParams` has **no `mcpServers` field** — it has
`dynamicTools?: Array<DynamicToolSpec>`. The inline entry was silently dropped, so the tool was
never surfaced via the model's tool list. (Real external MCP servers ride `config.toml`, read by
Codex natively — not `thread/start`.)

**Fix:** register `ask_user` via `dynamicTools` (`DynamicToolSpec = {name, description,
inputSchema, ...}`). `codexAskUserMcpServer` → `codexAskUserDynamicTool`; `codexThreadStartParams`
takes `dynamicTools` instead of the ignored `mcpServers`; the dead `defaultMcpServers` field was
removed.

### Defect B — missing experimentalApi capability

`dynamicTools` is gated: the app-server rejects it with JSON-RPC `-32600`
("thread/start.dynamicTools requires experimentalApi capability") unless the client opts in at
`initialize`.

**Fix:** `codexInitializeParams` now declares `capabilities:{experimentalApi:true,
requestAttestation:false}`. Verified it does not regress normal turns — the event mapper already
handles the v2 notification names (`turn/started`, `item/completed`, `turn/completed`) the
experimental API emits.

### Defect C — no handler for the dynamicTool call-back

When the model invokes a dynamicTool, the app-server sends a server→client request
`item/tool/call` with `DynamicToolCallParams = {threadId, turnId, callId, namespace, tool,
arguments}`. `handleInbound` only recognized approval/elicitation methods, so it replied
"unsupported Codex inbound request" and the call failed.

**Fix:** `handleInbound` now branches on `item/tool/call` → `handleDynamicToolCall`: parse
`arguments` (`codexAskUserArgs` → prompt/options/multiSelect), route to `bridge.AskQuestion`
(pause → `user_question_required` → options card → resume), and reply a `DynamicToolCallResponse`
(`codexDynamicToolResult` → `{contentItems:[{type:"inputText",text}], success}`). Unknown tool /
no bridge / a bridge error all reply `success:false` so the model gets a result and never hangs.
This branch is separate from — and leaves untouched — every approval branch.

### Follow-up — over-eager-question regression (nudge tuning)

Once `ask_user` worked, a test prompt with a clear file-write plus an ambiguous tail
(*"…and after that is all skills name i mentioned"*) made the model front-load clarifying
questions instead of creating the file and letting it flow through the approval gate. A live
spike confirmed the **approval code was intact** (a clear file-write prompt under YOLO=off fired
`item/commandExecution/requestApproval` ×2 and created the file; `ask_user` did not fire). The
cause was the `askUserReinforcement` nudge ("call ask_user … instead of guessing") biasing the
model to ask on any ambiguity.

**Fix:** softened `askUserReinforcement` (Codex) and `claudeAskUserReinforcement` (Claude) to:
"complete the clear, unambiguous parts directly — normal tools and approval gates still apply —
and only call `ask_user` when a required decision genuinely blocks you." No test asserts the
reinforcement text. Re-running the user's exact prompt with the softened nudge produced
`askedUser=0`, `approvals=3`, file created with the skill list — the clear action flowed through
the approval gate, no front-loaded question.

### Confirmed design (both providers)

With YOLO=true, all other tool/command calls auto-pass (Codex `danger-full-access`+`never`;
Claude `bypassPermissions`), but a structured `ask_user` Question is **still shown** to the user —
it routes through the user-interaction bridge (`user_question_required` → options card),
independent of the approval policy. Intended human-in-the-loop behavior, identical for Codex and
Claude.

## Verification

- `go build ./...` clean in `apps/local-runner`.
- New unit test `TestCodexAdapterAskUserDynamicToolRoundTrip`: `item/tool/call` → `bridge.AskQuestion`
  → `DynamicToolCallResponse{success:true, contentItems:[{inputText,"Go"}]}`.
- Real-codex end-to-end `TestCodexAskUserEndToEnd` (real `codex app-server` + production
  dispatcher/adapter; `FLOWPILOT_CODEX_E2E=1`, skipped by default): the model calls `ask_user` via
  `item/tool/call`, answer round-trips, turn completes. PASS in ~28.6s.
- No regression: 76 Codex + Claude + dispatcher + approval + policy + workflow-question tests pass.
  Approval round-trips (legacy + v2 + permissions + elicitation) are unchanged; `captureBridge` was
  extended only additively (zero-value default preserves old behavior); the ask_user registration
  test was updated from `mcpServers` to `dynamicTools` (the thing being fixed).
- Live spikes: (1) clear file-write under YOLO=off → approval flow fires, file created, `ask_user`
  does not fire — approvals intact; (2) the user's ambiguous prompt with the softened nudge →
  `askedUser=0`, `approvals=3`, file created.

## Residual Notes

- The earlier inline `mcpServers` shape was a long-standing assumption ("verify against the
  installed Codex build", 04-03/04-04) that the schema dump finally disproved — the registration
  channel is `dynamicTools`, the call-back is `item/tool/call`.
- `experimentalApi` is now on for ALL Codex turns (shared `initialize`). It is additive and the
  event mapper already handles the v2 notification names, so normal turns are unaffected.
- The working-tree change to `requirements/10-Refactor/New-System/08-Desktop-Chat-New-Plan.md` was
  NOT produced by this work and is left untouched for separate review.
- GitNexus interactive tools were not connected this session; edits relied on manual call-graph
  inspection plus the authoritative `codex app-server` JSON schema / TS bindings.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: TASK-056
change_type: fix
summary: Fix Codex ask_user Live Flow: dynamicTools + item/tool/call + Nudge Tuning (Task-056)
# --->8---
