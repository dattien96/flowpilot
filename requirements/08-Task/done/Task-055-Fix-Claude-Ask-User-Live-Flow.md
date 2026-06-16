# Task-055: Fix Claude Ask-User Live Flow (Structured Question Options Card)

## Metadata

- Document ID: `Task-055`
- Title: `Fix Claude Ask-User Live Flow (Structured Question Options Card)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [10-Refactor/07-Claude-Adapter-Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md), [10-Refactor/04-04-Phase4-Approval-Yolo-Finalizer](../../10-Refactor/New-System/04-04-Phase4-Approval-Yolo-Finalizer.md)
- Child Documents: `none`
- Related Documents: [CA-084: Fix Claude ask_user MCP connection gate + SSE](../../../change-audit/CA-084-fix-claude-ask-user-mcp-connection-gate-and-sse.md), [CA-083: Fix Claude FlowPilot MCP Availability](../../../change-audit/CA-083-fix-claude-flowpilot-mcp-availability-yolo-and-google-drive.md), [10-Refactor/06-DOD-And-Verification-Checklist](../../10-Refactor/New-System/06-DOD-And-Verification-Checklist.md)
- Replaces: `none`
- Tags: `runner, claude, mcp, ask_user, structured-question, approval-gate, yolo`

## AI Quick View

### Summary

- The structured-question `ask_user` MCP tool never reached the Claude model in the running desktop app: the QuestionCard options UI never appeared and the model fell back to asking in plain text.
- Three compounding causes: (A) Claude's **built-in** `AskUserQuestion` tool shadowed the FlowPilot MCP tool; (B) the FlowPilot `ask_user` MCP tool advertised an **empty input schema**; (C/primary) the runner-hosted MCP server **declined the GET SSE probe with 405**, so Claude's Streamable-HTTP client never marked the server connected and never exposed its tools, compounded by (D) the prompt being delivered before the **async, nonblocking** MCP connection completed.
- Fixed all four; the live model now calls `mcp__flowpilot__ask_user`, proven by a real-`claude` end-to-end test and confirmed in the desktop app.
- Confirmed desired UX: with YOLO=on every other tool auto-passes, but `ask_user` still surfaces the confirm/options card.

### Current Ask

- Done. Restart of `just dev` recompiles the runner (it launches via `go run`); the QuestionCard options card now renders for Claude in both YOLO states.

### Key Decisions

- `T-1` Disable the built-in `AskUserQuestion` tool (`--disallowed-tools AskUserQuestion`) so the model is forced onto `mcp__flowpilot__ask_user` (the built-in runs headless with no TTY and never reaches the FlowPilot bridge).
- `T-3` The runner-hosted MCP server MUST answer Claude's `GET` SSE probe with an open `text/event-stream`; 405 leaves the server stuck `pending` and its tools are never offered to the model.
- `T-4` Gate prompt delivery on the runner observing Claude's `tools/list` (connection live), bounded by a timeout so a slow/failed connect degrades to sending anyway rather than hanging.

### Constraints

- Runner-side only (Claude provider adapter + per-turn MCP server). No desktop UI contract changes — the existing QuestionCard already renders `user_question_required`.
- Must not regress the BUG-073 YOLO×MCP availability matrix or the YOLO permission-gating behavior (CA-083, CA-079, CA-081).
- Spawn-per-turn process model is unchanged (07 plan "Final Decision").

### Open Questions

- None for the Claude path. Codex's `ask_user` remains broken and is explicitly out of scope (see Section 7).

### Source Refs

- `PP-31`, `T-29` (10-Refactor 06-DOD: model-driven `ask_user` was `[~]` "best-effort, needs a live model" — this task closes that gap)
- `SS-08`, `SD-09` (Approval Gates & YOLO)
- `CA-083`, `CA-079`, `CA-081`

## 1. Goal

Make Claude's structured-question (`ask_user`) flow actually work in the running app: when the model needs a decision it calls `mcp__flowpilot__ask_user`, the runner emits `user_question_required`, and the desktop renders the selectable options card (QuestionCard) — in both YOLO=off and YOLO=on. This advances DOD `PP-31`/`T-29` from "registered but never observed calling" to verified-live.

## 2. Parent Links

- coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md); [10-Refactor/07-Claude-Adapter-Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md); [10-Refactor/04-04-Phase4-Approval-Yolo-Finalizer](../../10-Refactor/New-System/04-04-Phase4-Approval-Yolo-Finalizer.md)
- tech design: [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- system spec: [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)
- specific upstream ids: `PP-31`, `T-29` (10-Refactor 06-DOD), `CA-083`

## 3. Trigger

Manual testing of the live desktop app showed Claude never rendering the options card. The first test surfaced an "AskUserQuestion" approval card and "the user did not answer the questions" (the built-in tool, run headless). After disabling the built-in tool the model reported "I don't have access to an `ask_user` tool" — revealing that the FlowPilot MCP tool was never reaching the model at all, even though the `approve` permission tool on the same server worked. CA-083 had made the per-turn `--mcp-config` available in both YOLO states, but the live `ask_user` call was still broken by a deeper MCP connection-handshake defect.

## 4. Exact Change

- `T-1` `claudeArgs` adds `--disallowed-tools AskUserQuestion` in all YOLO states so the model cannot use the built-in tool and must call `mcp__flowpilot__ask_user`. (`claude_permission_mcp.go`)
- `T-2` `claudeMCPToolDefs` gives `ask_user` a real `inputSchema` (`prompt`/`options[]`/`multiSelect`, `required: [prompt]`), mirroring the Codex registration, so the model knows how to call it. (`claude_mcp_server.go`)
- `T-3` `claudeMCPServer.ServeHTTP` answers a `GET` with an open `text/event-stream` (`serveSSE`, keepalive comments until disconnect) instead of `405`, so Claude's Streamable-HTTP client marks the server connected and exposes its tools. (`claude_mcp_server.go`)
- `T-4` Per-token readiness gate: `register` creates a ready channel; `signalReady` closes it on the first `tools/list`; `waitReady(ctx, token, timeout)` blocks until connect / timeout / ctx-cancel. `SendTurn` withholds the user prompt until the gate fires (bounded by adapter `mcpReadyTimeout`, default `claudeMCPReadyDefaultTimeout = 10s`). (`claude_mcp_server.go`, `claude_adapter.go`)
- `T-5` Tests: built-in-disabled + schema-advertised unit tests, the readiness-gate unit test (unblock/timeout/ctx-cancel/unknown-token), the SSE `GET` unit test, and a real-`claude` end-to-end test gated behind `FLOWPILOT_CLAUDE_E2E=1`. Matrix test gets a short `mcpReadyTimeout` so its scripted fake process doesn't block on the gate. (`claude_permission_mcp_test.go`, `claude_mcp_server_test.go`, `claude_mcp_matrix_test.go`, `claude_e2e_test.go`)

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/claude_permission_mcp.go` (T-1)
  - `apps/local-runner/internal/runner/claude_mcp_server.go` (T-2, T-3, T-4)
  - `apps/local-runner/internal/runner/claude_adapter.go` (T-4: `mcpToken` hoist, `waitReady` gate, `mcpReadyTimeout` field)
  - `apps/local-runner/internal/runner/claude_permission_mcp_test.go` (T-5)
  - `apps/local-runner/internal/runner/claude_mcp_server_test.go` (T-5)
  - `apps/local-runner/internal/runner/claude_mcp_matrix_test.go` (T-5)
  - `apps/local-runner/internal/runner/claude_e2e_test.go` (T-5, new)
- modules: `local-runner` (Claude provider adapter + runner-hosted permission/ask_user MCP server)
- routes: runner MCP endpoint `ClaudeMCPPath` (`/internal/claude-permission-mcp`)
- tables: `none`

## 6. Acceptance Check

Verified this session (Go tests + real `claude` + live desktop app):

- `V-1` Built-in tool disabled in both YOLO states — `TestClaudeArgsDisablesBuiltinAskUserQuestion`.
- `V-2` `ask_user` advertises `prompt`/`options`/`multiSelect` with `required: [prompt]` — `TestClaudeAskUserToolAdvertisesSchema`.
- `V-3` Readiness gate unblocks on `tools/list`, times out without a connection, returns on ctx-cancel, false for unknown token — `TestClaudeMCPServerPromptGate` (4 subtests).
- `V-4` `GET` opens a `200 text/event-stream` with the open-stream prelude — `TestClaudeMCPGetOpensSSEStream`.
- `V-5` **Real-`claude` end-to-end** (real adapter + real Go MCP server + real CLI): the model calls `mcp__flowpilot__ask_user` ("Which language would you like to use: Python or Go?") — `TestClaudeAskUserEndToEnd` (`FLOWPILOT_CLAUDE_E2E=1`, skipped by default; PASS in 8.2s).
- `V-6` **No regression — command-write + MCP availability matrix, YOLO on/off** — `TestClaudeSendTurnMcpAvailabilityMatrix` (BUG-073) and `TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade` pass.
- `V-7` **No regression — MCP call YOLO on/off** — `TestClaudeArgsYoloPosture` / `TestClaudeArgsIncludesStrictMcpConfig` pass.
- `V-8` **Whole-package regression check** — `go test ./internal/runner` shows the **same 23 pre-existing failures with and without these changes** (verified by `git stash`), all environmental (Google Drive OAuth/credentials/launcher absent, skills-source env). No new failures; `go build ./...` clean.
- `V-9` **Live desktop app** (user-confirmed after `just dev` restart): the QuestionCard options card renders for Claude; with **YOLO=on every other tool call auto-passes, but `ask_user` still surfaces the confirm/options card** — the intended human-in-the-loop behavior.

* all case test with command write tool + mcp tools with YOLO on and off passed. No regression
* Test for mcp call with yolo on/off passed. no regression
* new test for ask user tool passed for claude
* 1 thing very good is, even YOLO = on. all other call must auto pass. but ask-user still show to confirm. This is exactly what i want

## 7. Out of Scope

- **Codex `ask_user`** — still broken and NOT fixed here. Codex uses a different architecture (inline tool registration on `thread/start`, no runner-hosted HTTP MCP, and `handleInbound` has no `tools/call` handler). The SSE/connection-gate fixes do not apply. Requires a separate spike to capture the codex-cli app-server's real tool-call wire frame, then a registration-shape fix + inbound handler.
- The `requirements/10-Refactor/New-System/08-Desktop-Chat-New-Plan.md` change present in the working tree was NOT made by this task and is left untouched for separate review.
- Admin Web question-history timeline and any `provider_questions` Supabase migration (Phase 8 cutover items).
- No desktop UI changes — the existing QuestionCard already handles `user_question_required`.

## 8. Completion Notes

- result: Claude's `ask_user` structured-question flow works end-to-end. Root cause was the runner-hosted MCP server declining Claude's GET SSE probe (server stuck `pending`, tools never exposed) plus the prompt being sent before the async MCP connection completed, with the built-in `AskUserQuestion` tool shadowing the MCP tool and an empty `ask_user` schema as contributing factors. Closes the live-model gap left open in DOD `T-29`/`PP-31`.
- follow-ups: Fix the Codex `ask_user` path (separate spike + task). Optionally promote DOD `T-29` from `[~]` to verified-live, and `T-40` (live `ask_user` tool-result on expiry) now that the live MCP path is proven.
- upstream docs updated: `Task-055`, `CA-084`. GitNexus interactive tools were not connected this session; edits relied on manual call-graph inspection (`SendTurn` → `claudeArgs` / `writeClaudeMCPConfig` → `claudeMCPServer.dispatch`/`serveSSE`/`waitReady`).
