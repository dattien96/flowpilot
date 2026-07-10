# BUG-273: Grok Does Not Gate MCP Tool Calls Under YOLO=Off In Chat

## Metadata

- Document ID: `BUG-273`
- Title: `Grok Does Not Gate MCP Tool Calls Under YOLO=Off In Chat`
- Phase: `bugfix`
- Status: `in_progress` — root cause diagnosed by code trace 2026-07-10; the exact fix requires a live investigation of Grok's ACP MCP-trust behavior (why it does not emit `session/request_permission` for external MCP tool calls). No code changed yet.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [Task-209: Grok MCP Ask-User Spawn-Agent Parity](../../08-Task/inprogress/Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md), [Task-208: Grok Permission Channel And YOLO Posture](../../08-Task/done/Task-208-Grok-Permission-Channel-And-Yolo-Posture.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)
- Child Documents: `none`
- Related Documents: [Task-218: Grok YOLO Enforced Via Config Rewrite And Always-Approve Flag](../../08-Task/done/Task-218-Grok-Yolo-Enforced-Via-Config-Rewrite-And-Always-Approve-Flag.md)
- Replaces: `none`
- Tags: `ai-providers`, `mcp-tools`, `grok`, `yolo-policy`, `approval`, `regression`

## AI Quick View

### Summary

- With YOLO=off, Grok gates its **native** tools (file/exec) correctly — `session/request_permission` → `RequestApproval` → standard approval card (Task-208, live-confirmed). But Grok tool calls to **external MCP servers** (e.g. Google Drive, and FlowPilot's own `ask_user`) run **without any gate** under YOLO=off.
- Claude gates MCP tool calls in chat via its `--permission-prompt-tool` MCP server (every tool, including MCP, routes through `RequestApproval` → the same approval card). Grok has no equivalent — it appears to auto-trust connected MCP servers and never emits `session/request_permission` for their tools.
- The Google Drive proxy's own internal gate is the only backstop for Grok, and it does not engage in the interactive chat path: (a) READ tools auto-approve on `s.yoloMode || s.hasNoApprovalScope()` and the interactive path never wires the proxy approval scope, and (b) even if it created a pending record, the desktop chat has no UI to surface/resolve google-drive-proxy approvals (that flow exists only in admin-web/workflow).

### Current Ask

- Make Grok gate external MCP tool calls under YOLO=off through the standard approval card (same path as its native tools), so YOLO=false is enforced for MCP just like it is for file/exec — without introducing a hang.

### Key Decisions

- `V-1` The fix must route Grok MCP tool approvals through the existing `TurnBridge.RequestApproval` → standard approval card (resolved by `SubmitApprovalDecision`), NOT through the Google Drive proxy's file-based approval store — the interactive desktop has no UI for the latter, so a proxy pending-record would hang the turn (worse than the current auto-run).
- `V-2` "Cách A" (thread the proxy approval scope `FLOWPILOT_WORKFLOW_RUN_ID`/`STEP`/`PROCESS_KEY` into Grok's proxy MCP config) was evaluated and **rejected for the chat path**: it depends on the admin-web-only proxy-approval UI and would hang the interactive turn. Documented so it is not re-attempted.
- `V-3` The real lever is Grok's ACP MCP-trust behavior: determine why it does not emit `session/request_permission` for MCP tool calls and change that (config, per-server trust, permission flag, or a `PreToolUse`-style hook) so MCP tools reach the same permission channel as native tools. This needs live investigation against the real `grok` binary (the captured `testdata/grok_acp` probe only exercised a native read-only tool).

### Constraints

- Do not weaken Claude/Codex/Gemini MCP gating.
- Do not create pending state the interactive desktop cannot resolve (no hang).
- Additive to the Grok adapter / its MCP wiring; keep within Task-209's parity scope.

### Open Questions

- `Q-1` **ANSWERED (live 2026-07-10):** Grok does NOT emit `session/request_permission` for MCP tool calls under `permission_mode="default"` — nor for read-class native tools. Only write/exec native tools prompt. So the permission channel is not a viable gate for MCP tools; a Grok `PreToolUse`-style hook or a FlowPilot-side interception is required.
- `Q-2` (open) Does an MCP **write** tool prompt under `default` (i.e. is the auto-run read-class-only, or all-MCP)? The probe only exercised a read-ish MCP call (`authGetStatus`). Cheap to check via the desktop: a Drive **write** under YOLO=off — if it also auto-runs, it is all-MCP-auto; if it prompts, it is read-class-auto.
- `Q-3` Does a Grok Claude-Code-style `PreToolUse` hook (grok supports hooks) fire for `CallMcpTool`, and can it block/defer pending a FlowPilot approval? This is the most promising fix lever.

### Source Refs

- `apps/local-runner/internal/runner/grok_adapter.go` (`handleInbound` — native `session/request_permission` routing; MCP tools do not arrive here today), `grok_process.go` (MCP env, trust flags `GROK_CLAUDE_MCPS_ENABLED`/`GROK_CURSOR_MCPS_ENABLED`).
- `google_drive_proxy_mcp.go` (`handleReadTool` line ~494 `s.yoloMode || s.hasNoApprovalScope()`; `hasApprovalScope` needs run/step/process key), `google_drive_mcp_provider_config.go` (`flowpilotClaudeExtraMCPServers` builds the proxy with empty scope for both providers).
- `claude_permission_mcp.go` (Claude's `--permission-prompt-tool` — the mechanism Grok lacks for MCP).
- Live report 2026-07-10: `Using mcp google drive to let me know the current google email` — Claude shows the approval card (correct); Grok auto-runs. A non-MCP command (`Create a file …`) does gate for Grok.

## 1. Issue Summary

Under YOLO=off, Grok runs external MCP tool calls (Google Drive, ask_user) without any approval prompt, while the same account correctly gates native file/exec tools and while Claude correctly gates the MCP tool.

## 2. Parent Links

- impacted coding plan: `CP-46` (Grok controlled adapter over ACP)
- impacted tech design: `SD-09` (Approval Gates)
- impacted system spec: `SS-08` (YOLO is the SSOT; YOLO=off must gate)

## 3. Environment and Reproduction

- environment: FlowPilot desktop + local runner, Grok provider, YOLO toggle off, a Google Drive MCP configured for the account.
- reproduction steps: with YOLO off, send `Using mcp google drive to let me know the current google email of this drive`.
- frequency: deterministic.

## 4. Expected vs Actual

- expected: an approval card appears (same as Claude, same as Grok's native-tool gate) and the MCP tool runs only after approval.
- actual: Grok executes the MCP tool immediately, no card.

## 5. Impact

- users affected: Grok users relying on YOLO=off to review MCP/Drive actions.
- workflows affected: any Grok chat using MCP tools (Drive read/write, ask_user).
- severity: medium-high — a YOLO=off safety guarantee (SS-08) silently does not hold for MCP tools on Grok. Read-only Drive today, but the same gap would apply to write/destructive MCP tools.

## 6. Root Cause

- confirmed (code trace): Grok only sends `session/request_permission` (→ `RequestApproval` → card) for its native tools. External MCP tool calls never reach `handleInbound`, so FlowPilot's runner gate never sees them. Claude gates all tools (incl. MCP) via `--permission-prompt-tool`; Grok has no equivalent.
- confirmed (code trace): the Google Drive proxy's own gate cannot cover the chat path — READ auto-approves on `hasNoApprovalScope()` (the interactive path wires no scope), and the desktop chat has no UI to surface/resolve proxy pending approvals (admin-web/workflow-only).
- **CONFIRMED LIVE 2026-07-10** (scripted `grok agent stdio` ACP probe, real turn, `permission_mode="default"` from config): a prompt that read a native file (`Glob`, `Read`) and then called the Google Drive MCP tool (`CallMcpTool` → `google-drive` → `authGetStatus`) produced **zero** `session/request_permission` frames — all three tool calls executed directly. So under `default`, Grok auto-runs BOTH read-class native tools AND MCP tool calls; only write/exec native tools prompt (matching the observed create-file gate). Grok wraps MCP calls in a native `CallMcpTool` tool that its permission model does not gate under `default`. This is Grok's own Claude-Code-derived permission model (reads are auto-safe), not a FlowPilot wiring gap — FlowPilot's runner gate never gets a chance because no permission request is emitted.
- CLI levers found (none is a clean "prompt for MCP too" switch): `grok agent` exposes only `--always-approve` (no `--permission-mode`; that flag exists on the interactive `grok` TUI, values `default|acceptEdits|auto|dontAsk|bypassPermissions|plan`, but none means "ask for reads/MCP too"). `--allow`/`--deny` are Claude-style tool-pattern rules (`--deny` blocks, does not turn into a prompt). `grok mcp add` has no per-server trust/approval flag. `grok inspect` shows permission rules empty (`Source: (none)`).

## 7. Fix Strategy

- `F-1` **(pending live investigation)** Make Grok emit a permission request for MCP tool calls under YOLO=off, or otherwise intercept them, so they route through `TurnBridge.RequestApproval` and render the standard approval card. Candidate levers to test live: Grok permission/trust config for MCP servers, a Grok `PreToolUse`-style hook, or FlowPilot-side interception at the HTTP-MCP boundary for its own tools.
- `F-2` Explicitly NOT the Google Drive proxy scope wiring (rejected, `V-2`).

## 8. Validation

- `V-4` **(pending)** With YOLO=off, a Grok MCP tool call surfaces the standard approval card; deny blocks it, approve runs it.
- `V-5` **(pending)** YOLO=true still auto-runs MCP tools (no regression to the Task-218 bypass).
- `V-6` No regression to Claude/Codex/Gemini MCP gating.

## 9. Regression Guard

- tests: to be added once the live mechanism is known (adapter-level test that an MCP tool call under YOLO=off reaches `RequestApproval`).
- audit checks: Grok native-tool gating (Task-208 tests) must stay green.

## 10. Follow-Up Document Updates

- upstream docs that must change: Task-209 (fold the MCP-permission-parity finding into its scope once fixed).
- notes left unchanged on purpose: the Google Drive proxy's admin-web approval flow is unchanged; this bug is specifically the interactive-chat Grok MCP gate.
