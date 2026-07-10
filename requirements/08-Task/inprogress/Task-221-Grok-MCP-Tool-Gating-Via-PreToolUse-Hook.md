# Task-221: Grok MCP Tool Gating Via PreToolUse Hook

## Metadata

- Document ID: `Task-221`
- Title: `Grok MCP Tool Gating Via PreToolUse Hook`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [BUG-273: Grok Does Not Gate MCP Tool Calls Under YOLO=Off In Chat](../../09-BugFix/inprogress/BUG-273-Grok-Does-Not-Gate-MCP-Tool-Calls-Under-Yolo-Off-In-Chat.md), [Task-208: Grok Permission Channel And YOLO Posture](./done/Task-208-Grok-Permission-Channel-And-Yolo-Posture.md), [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Child Documents: `None`
- Related Documents: [Task-209: Grok MCP Ask-User Spawn-Agent Parity](./Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md) (wires the MCP tools; this task gates them), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- Replaces: `None`
- Tags: `grok, mcp, yolo-policy, approval, hook, acp, ai-providers`

## AI Quick View

### Summary

- BUG-273 confirmed live: under `permission_mode="default"`, Grok emits `session/request_permission` only for write/exec **native** tools. Read-class native tools and ALL external MCP tool calls (wrapped by Grok as the native `CallMcpTool`) run with NO permission request — so FlowPilot's runner gate never sees them and YOLO=off does not gate MCP (Google Drive, ask_user, etc.).
- Grok's ACP has no flag to change this (`grok agent` exposes only `--always-approve`; `--allow`/`--deny` are block/allow rules, not "turn into a prompt"). The remaining lever is a Grok **`PreToolUse` hook** (Grok supports Claude-Code-style hooks) that fires before a tool executes and can block/defer it.
- This task: build a FlowPilot-owned Grok `PreToolUse` hook that intercepts `CallMcpTool` (and, decision pending, other non-gated tools) under YOLO=off, routes the decision through the existing `TurnBridge.RequestApproval` → standard approval card, and lets deny block / approve proceed — reaching parity with how Claude gates MCP via `--permission-prompt-tool`.

### Current Ask

- Prove, live, that a Grok MCP tool call under YOLO=off surfaces the standard approval card (deny blocks, approve runs), YOLO=true still auto-runs, and Codex/Claude/Gemini are untouched.

### Key Decisions

- `T-1` Gate through the EXISTING `TurnBridge.RequestApproval` → standard approval card (resolved via `SubmitApprovalDecision`), NOT the Google Drive proxy's file-based approval store — the interactive desktop has no UI for the latter and a proxy pending-record would hang the turn (BUG-273 `V-1`/`V-2`).
- `T-2` Mechanism: a Grok `PreToolUse` hook (verify the exact hook contract live first — command hook vs. HTTP; what payload it receives for `CallMcpTool`; how it signals allow/deny/ask). The hook must call back into the running FlowPilot runner (HTTP, mirroring the runner-hosted MCP base URL) so it can reach the live turn's bridge; a hook that only returns allow/deny synchronously must block until FlowPilot resolves the approval.
- `T-3` Scope the gate to YOLO=off only: under YOLO=true the hook must fast-path allow (no card), consistent with Task-218 `--always-approve`. Read-class native tools already run without a hook today; decide whether the hook also covers them or only `CallMcpTool` (BUG-273 `Q-2` — is MCP-write also auto-run? informs whether reads need covering too).
- `T-4` Only install the hook for FlowPilot-launched Grok processes (per-process, via `--plugin-dir`/hook config the runner writes), never mutating the user's global `~/.grok` hook config (mirror the Task-218 "don't clobber user config" discipline where possible; if the hook must live in config.toml, follow the same minimal-rewrite + restore approach).

### Constraints

- **ZERO BASE REGRESSION (CP-46 `P-0`):** additive to the Grok path only; do not touch Claude/Codex/Gemini gating or the shared bridge/approval code beyond reuse.
- Must not hang a turn: every intercepted tool call must reach a definite allow/deny (bridge decision, YOLO fast-path, or a bounded timeout that fails safe).
- Must not reintroduce ambient MCP/compat scanning disabled by Task-206.
- YOLO=true behavior (auto-run) must not regress.

### Open Questions

- `Q-1` What exactly is Grok's `PreToolUse` hook contract over `grok agent stdio` — invocation shape, payload for `CallMcpTool`, and the allow/deny/ask response protocol? (Needs a live probe against the real binary; the captured `testdata/grok_acp` probe never exercised hooks.)
- `Q-2` Can a hook BLOCK synchronously long enough for a human approval (seconds–minutes), or does it need an async defer/re-ask protocol? If it can only allow/deny fast, we need a different interception point.
- `Q-3` Does the hook see read-class native tools too, letting us optionally gate those under YOLO=off, or only `CallMcpTool`? (Ties to BUG-273 `Q-2`.)
- `Q-4` Fallback if hooks are unsuitable: intercept at FlowPilot's own MCP boundary for FlowPilot-hosted tools (ask_user/spawn_agent already bridge-gated), and for external servers (Google Drive) front them through a FlowPilot proxy that calls the bridge — larger, only if `PreToolUse` proves unworkable.

### Source Refs

- `BUG-273` (live findings, CLI-lever survey), `Task-208` (native-tool `session/request_permission` → bridge path to mirror), `Task-218` (YOLO posture, `--always-approve`, config-rewrite discipline).
- `apps/local-runner/internal/runner/grok_adapter.go` (`handleInbound`, session wiring), `grok_process.go` (process launch, `--always-approve`, config), `interactive_service.go` (`RequestApproval`/`SubmitApprovalDecision` bridge), `claude_permission_mcp.go` (the Claude mechanism this achieves parity with).
- Live evidence 2026-07-10: scripted `grok agent stdio` probe — `CallMcpTool` executed with zero `session/request_permission` under `permission_mode="default"`.

## 1. Goal

Under YOLO=off, make Grok's external MCP tool calls (Google Drive, and any MCP server) surface the standard FlowPilot approval card and honor deny/approve — closing the BUG-273 gate gap and reaching Claude-level MCP gating parity, without hanging turns or touching other providers.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-09`
- system spec: `SS-08`
- specific upstream ids: `BUG-273`, `Task-208 T-1..T-4`

## 3. Trigger

BUG-273 proved live that Grok never asks permission for MCP tool calls, so YOLO=off silently fails to gate MCP actions. Task-208 (native-tool gating) and Task-209 (MCP tool wiring) are both done/in-flight but neither gates MCP; a dedicated mechanism is needed.

## 4. Exact Change

- `T-1` **(spike first)** Live-probe Grok's `PreToolUse` hook contract over `grok agent stdio` (answer `Q-1`/`Q-2`): payload for `CallMcpTool`, allow/deny/ask response shape, whether it can block for a human decision.
- `T-2` Build a FlowPilot Grok hook (per-process install) that, under YOLO=off, intercepts `CallMcpTool` and calls back to the runner → `TurnBridge.RequestApproval` → standard approval card; approve ⇒ allow, deny ⇒ block.
- `T-3` YOLO=true fast-path: hook allows immediately (no card), consistent with `--always-approve`.
- `T-4` Install the hook only for FlowPilot-launched Grok processes; do not clobber the user's global hook config (minimal-rewrite + restore if config.toml is the only option).
- `T-5` Tests: adapter/unit coverage that a `CallMcpTool` under YOLO=off reaches `RequestApproval` (deny blocks, approve proceeds) and YOLO=true does not; a live opt-in test against the real binary (mirror `grok_live_chat_test.go` gating).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/grok_process.go` (hook install at launch), `grok_adapter.go` (hook callback → bridge), a new hook-callback HTTP handler (or reuse the runner MCP base URL), tests; `requirements/07-Coding-Plan/.../CP-46` (status).
- modules: Grok adapter, runner approval bridge, process launch/config.
- routes: possibly one new runner endpoint for the hook callback (or reuse the MCP server).
- tables: none.

## 6. Acceptance Check

- YOLO=off: a Grok Google Drive (and generic MCP) tool call surfaces the standard approval card; deny prevents execution; approve executes.
- YOLO=true: MCP tools auto-run, no card (no Task-218 regression).
- Native write/exec gating (Task-208) and Claude/Codex/Gemini gating unchanged.
- No turn hangs; failure modes fail safe (deny/bounded timeout).

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` Grok `PreToolUse` hook contract documented from a live probe (`Q-1`/`Q-2` answered).
- [ ] `DOD-2` Hook intercepts `CallMcpTool` under YOLO=off and routes to `RequestApproval` → standard card; deny blocks, approve runs (unit + live).
- [ ] `DOD-3` YOLO=true fast-path allow proven (no card).
- [ ] `DOD-4` Hook installed per-process only; user global hook config not clobbered.
- [ ] `DOD-5` No regression: Task-208 native gating, Task-218 YOLO, Claude/Codex/Gemini MCP gating all green.

## 7. Out of Scope

- Gating read-class **native** tools (decide in `T-3`/`Q-3`; only if cheap and the hook covers them).
- The Google Drive proxy's admin-web approval flow (unchanged).
- Any change to how MCP tools are *wired* to Grok — that is Task-209.

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
