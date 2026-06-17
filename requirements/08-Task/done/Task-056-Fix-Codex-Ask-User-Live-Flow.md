# Task-056: Fix Codex Ask-User Live Flow (Dynamic Tool Structured Question)

## Metadata

- Document ID: `Task-056`
- Title: `Fix Codex Ask-User Live Flow (Dynamic Tool Structured Question)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [10-Refactor/05-Codex-AppServer-Migration-Detail](../../10-Refactor/New-System/05-Codex-AppServer-Migration-Detail.md), [10-Refactor/04-04-Phase4-Approval-Yolo-Finalizer](../../10-Refactor/New-System/04-04-Phase4-Approval-Yolo-Finalizer.md)
- Child Documents: `none`
- Related Documents: [Task-055: Fix Claude Ask-User Live Flow](../done/Task-055-Fix-Claude-Ask-User-Live-Flow.md), [CA-085: Fix Codex ask_user dynamicTool + nudge tuning](../../../change-audit/CA-085-fix-codex-ask-user-dynamic-tool-and-nudge.md), [CA-084: Fix Claude ask_user MCP connection gate + SSE](../../../change-audit/CA-084-fix-claude-ask-user-mcp-connection-gate-and-sse.md), [10-Refactor/06-DOD-And-Verification-Checklist](../../10-Refactor/New-System/06-DOD-And-Verification-Checklist.md)
- Replaces: `none`
- Tags: `runner, codex, app-server, ask_user, dynamic-tool, structured-question, approval-gate, yolo`

## AI Quick View

### Summary

- The structured-question `ask_user` tool never reached the Codex model: it reported "I can't access the interactive user prompt tool" and answered in plain text.
- Three causes, verified against codex-cli 0.140.0 via `codex app-server generate-json-schema` + live spikes: (A) `ask_user` was registered as an inline `mcpServers:[{name,tools:[]}]` entry, but `ThreadStartParams` has **no `mcpServers` field** — silently ignored; the real channel is `dynamicTools:[DynamicToolSpec]`; (B) `initialize` did not request `capabilities.experimentalApi=true`, which `dynamicTools` requires (else JSON-RPC -32600); (C) the model's call returns as a server→client `item/tool/call` (`DynamicToolCallParams`) that `handleInbound` had no handler for.
- Fixed all three (declare experimentalApi, register via dynamicTools, handle `item/tool/call` → `bridge.AskQuestion` → `DynamicToolCallResponse`). Proven by a real-codex end-to-end test.
- Follow-up tuning: softened the `askUserReinforcement` nudge (Codex **and** Claude) so the model completes clear actions first (normal approval gates apply) and only calls `ask_user` when genuinely blocked — fixing an over-eager-question regression.

### Current Ask

- Done. Restart of `just dev` recompiles the runner (`go run`); Codex now surfaces the QuestionCard for `ask_user`, and clear actions still flow through the normal approval gate.

### Key Decisions

- `T-1` `initialize` MUST declare `capabilities.experimentalApi=true`; without it the app-server rejects `dynamicTools` with -32600.
- `T-2` Register `ask_user` as a thread `dynamicTool` (`DynamicToolSpec`) — the inline `mcpServers` shape is not a `ThreadStartParams` field and is silently dropped.
- `T-3` The model's dynamicTool call arrives as `item/tool/call`; route it to the user-interaction bridge and reply a `DynamicToolCallResponse` (never an "unsupported" error).
- `T-4` Bias the ask_user reinforcement toward acting: complete clear work first (approval gates apply) and only ask when genuinely blocked.

### Constraints

- Runner-side only (Codex provider adapter + app-server param builders + the shared ask_user reinforcement). No desktop UI contract changes.
- MUST NOT regress the YOLO / command-tool / MCP-tool / approval tests: the approval round-trips stay byte-for-byte unchanged; `item/tool/call` is a new, separate branch.
- Spawn-per-turn / shared app-server process model unchanged.

### Open Questions

- None. Both Codex and Claude `ask_user` paths are verified-live.

### Source Refs

- `PP-31`, `T-29`, `T-30`, `T-40` (10-Refactor 06-DOD: model-driven `ask_user` + workflow-driven question; T-29/T-40 were deferred pending a live model/MCP — this closes the Codex side)
- `SS-08`, `SD-09` (Approval Gates & YOLO)
- `Task-055`, `CA-084`, `CA-085`

## 1. Goal

Make Codex's structured-question (`ask_user`) flow work in the running app: when the model needs a decision it calls the `ask_user` dynamicTool, the runner emits `user_question_required`, and the desktop renders the selectable options card (QuestionCard) — in both YOLO states — without regressing the command/file approval flow. Sibling of Task-055 (Claude); advances DOD `PP-31`/`T-29` for Codex.

## 2. Parent Links

- coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md); [10-Refactor/05-Codex-AppServer-Migration-Detail](../../10-Refactor/New-System/05-Codex-AppServer-Migration-Detail.md); [10-Refactor/04-04-Phase4-Approval-Yolo-Finalizer](../../10-Refactor/New-System/04-04-Phase4-Approval-Yolo-Finalizer.md)
- tech design: [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- system spec: [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)
- specific upstream ids: `PP-31`, `T-29`, `T-30` (10-Refactor 06-DOD), `Task-055`

## 3. Trigger

Live desktop testing showed Codex replying *"I can't access the interactive user prompt tool"* for an `ask_user` prompt — the tool was never surfaced. After fixing it, a follow-up test (*"Create a file yolo-test.txt with 'yolo works' and after that is all skills name i mentioned"*) showed the model front-loading clarifying questions where the user expected the file write to flow through the normal approval card — traced to the over-eager `askUserReinforcement` nudge (the approval code itself was proven intact).

## 4. Exact Change

- `T-1` `codexInitializeParams` declares `capabilities:{experimentalApi:true, requestAttestation:false}` (required for `dynamicTools`). (`codex_appserver.go`)
- `T-2` `codexThreadStartParams` registers `ask_user` via `dynamicTools` (a `DynamicToolSpec`) instead of the ignored inline `mcpServers` shape; `codexAskUserMcpServer` → `codexAskUserDynamicTool`; dead `defaultMcpServers` field removed. (`codex_appserver.go`, `codex_adapter.go`)
- `T-3` `handleInbound` handles method `item/tool/call` → `handleDynamicToolCall`: parse `DynamicToolCallParams.arguments` (`codexAskUserArgs`), route to `bridge.AskQuestion`, reply `DynamicToolCallResponse` (`codexDynamicToolResult` → `{contentItems:[{type:"inputText",text}], success}`); unknown tool / no bridge / error reply `success:false` so the model never hangs. (`codex_adapter.go`)
- `T-4` Softened `askUserReinforcement` (Codex) **and** `claudeAskUserReinforcement` (Claude): complete the clear, unambiguous parts directly (normal tools + approval gates apply) and only call `ask_user` when a required decision genuinely blocks progress. (`codex_adapter.go`, `claude_adapter.go`)
- `T-5` Tests: `TestCodexAdapterAskUserDynamicToolRoundTrip` (unit, mirrors approval round-trips); real-codex `TestCodexAskUserEndToEnd` (`FLOWPILOT_CODEX_E2E=1`, skipped by default); `captureBridge` extended additively with ask_user capture fields; the ask_user registration assertion updated from `mcpServers` to `dynamicTools`. (`codex_appserver_test.go`, `codex_e2e_test.go`, `partd_test.go`)

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/codex_appserver.go` (T-1, T-2)
  - `apps/local-runner/internal/runner/codex_adapter.go` (T-2, T-3, T-4 Codex nudge, removed `defaultMcpServers`, `strings` import)
  - `apps/local-runner/internal/runner/claude_adapter.go` (T-4 Claude nudge)
  - `apps/local-runner/internal/runner/codex_appserver_test.go` (T-5)
  - `apps/local-runner/internal/runner/partd_test.go` (T-5)
  - `apps/local-runner/internal/runner/codex_e2e_test.go` (T-5, new)
- modules: `local-runner` (Codex provider adapter + app-server JSON-RPC builders; shared ask_user reinforcement)
- routes: Codex app-server JSON-RPC (`initialize`, `thread/start`, `turn/start`, inbound `item/tool/call`)
- tables: `none`

## 6. Acceptance Check

Verified this session (Go tests + real `codex app-server` + live spikes):

- `V-1` `ask_user` is registered as a thread `dynamicTool` (`TestCodexAdapterRegistersAskUserAndUsesReqCwd` now asserts `dynamicTools` contains `ask_user`).
- `V-2` An `item/tool/call` for `ask_user` routes its `arguments` to `bridge.AskQuestion` and replies a `DynamicToolCallResponse` (`success:true`, `contentItems[0]={type:"inputText",text}`) — `TestCodexAdapterAskUserDynamicToolRoundTrip`.
- `V-3` **Real-codex end-to-end**: the model calls `ask_user` via `item/tool/call` and the answer round-trips — `TestCodexAskUserEndToEnd` (`FLOWPILOT_CODEX_E2E=1`; PASS in ~28.6s).
- `V-4` **No regression**: all Codex approval round-trips (legacy `execCommandApproval`/`applyPatchApproval`, v2 `item/commandExecution|fileChange/requestApproval`, `item/permissions/requestApproval`, `mcpServer/elicitation/request`) plus the Claude suite pass — 76 Codex+Claude+dispatcher+approval+policy+workflow-question tests green. The approval branches are unchanged; `captureBridge` was extended only additively.
- `V-5` **Approval flow intact (live spike)**: a clear file-write prompt under YOLO=off (`workspace-write`+`untrusted`) with `ask_user` registered fired `item/commandExecution/requestApproval` ×2 and created the file; `ask_user` did **not** fire (`askedUser=false`). `dynamicTools` does not hijack approvals.
- `V-6` **Over-eager-question fix (live spike)**: the user's exact ambiguous prompt with the softened nudge produced `askedUser=0`, `approvals=3`, file created with `yolo works` + the skill list — the clear action flowed through the approval gate instead of front-loading a question.

## 7. Out of Scope

- No desktop UI changes — the existing QuestionCard already renders `user_question_required` for both providers.
- Admin Web question-history timeline and any `provider_questions` Supabase migration (Phase 8 cutover items).
- The working-tree change to `requirements/10-Refactor/New-System/08-Desktop-Chat-New-Plan.md` was NOT made by this task and is left untouched for separate review.

## 8. Completion Notes

- result: Codex `ask_user` works end-to-end via the `dynamicTools` channel + `item/tool/call` handler, and the softened reinforcement keeps the model acting on clear work (approval gates apply) instead of over-asking. **Confirmed design (Codex and Claude alike): with YOLO=true all other tool/command calls auto-pass (Codex `danger-full-access`+`never`; Claude `bypassPermissions`), but a structured `ask_user` Question is STILL shown to the user** — because it routes through the user-interaction bridge (`user_question_required` → options card), independent of the approval policy. This intended human-in-the-loop behavior is the same for both providers.
- follow-ups: Optionally promote DOD `T-29`/`T-40` to verified-live for Codex. The un-owned `08-Desktop-Chat-New-Plan.md` change still needs review.
- upstream docs updated: `Task-056`, `CA-085`. GitNexus interactive tools were not connected this session; edits relied on manual call-graph inspection (`SendTurn` → `codexThreadStartParams` / `codexAskUserDynamicTool`; dispatcher inbound → `handleInbound` → `handleDynamicToolCall` → `bridge.AskQuestion`) plus the authoritative app-server JSON schema.
