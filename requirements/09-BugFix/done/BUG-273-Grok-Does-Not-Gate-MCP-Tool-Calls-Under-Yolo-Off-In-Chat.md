# BUG-273: Grok Does Not Gate MCP Tool Calls Under YOLO=Off In Chat

## Metadata

- Document ID: `BUG-273`
- Title: `Grok Does Not Gate MCP Tool Calls Under YOLO=Off In Chat`
- Phase: `bugfix`
- Status: `done` — closed 2026-07-11: product gap no longer reproducible on desktop; fixed by existing Task-208 permission channel + Task-218 YOLO posture (no PreToolUse). Task-221 cancelled as won't-do.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-11`
- Parent Documents: [Task-208: Grok Permission Channel And YOLO Posture](../../08-Task/done/Task-208-Grok-Permission-Channel-And-Yolo-Posture.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md) — this is a permission/YOLO-gating gap (Task-208 lineage), not a tool-wiring gap.
- Child Documents: [Task-221: Grok MCP Tool Gating Via PreToolUse Hook](../../08-Task/done/Task-221-Grok-MCP-Tool-Gating-Via-PreToolUse-Hook.md) (cancelled / won't-do PreToolUse)
- Related Documents: [Task-209: Grok MCP Ask-User Spawn-Agent Parity](../../08-Task/done/Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md) (tool wiring / prompt reinforcement — separate; does **not** create Drive ApprovalCards), [Task-218: Grok YOLO Enforced Via Config Rewrite And Always-Approve Flag](../../08-Task/done/Task-218-Grok-Yolo-Enforced-Via-Config-Rewrite-And-Always-Approve-Flag.md)
- Replaces: `none`
- Tags: `ai-providers`, `mcp-tools`, `grok`, `yolo-policy`, `approval`, `regression`

## AI Quick View

### Summary

- **Initial report (2026-07-10):** under YOLO=off, Grok gated native write/exec but appeared to auto-run Google Drive MCP without an ApprovalCard; a scripted ACP probe saw zero `session/request_permission` for `CallMcpTool`.
- **Resolution (2026-07-11):** user desktop re-verification shows Grok **does** gate Drive MCP via the standard ApprovalCard when YOLO is off. Mechanism is Task-208 (`session/request_permission` → `RequestApproval`) with Task-218 keeping `permission_mode=default` under YOLO=false. Task-221 PreToolUse **won't-do**.
- Working path is **not** the Google Drive proxy approval store and **not** Task-209 prompt reinforcement.

### Current Ask

- None — closed. Reopen if MCP permission emission regresses or non-Drive MCP fails the same YOLO checks.

### Key Decisions

- `V-1` Route Grok MCP approvals through `TurnBridge.RequestApproval` → standard ApprovalCard — **confirmed working** for Drive on desktop 2026-07-11.
- `V-2` Proxy approval-scope wiring for chat path remains **rejected** (would hang without admin-web UI).
- `V-3` PreToolUse lever **not required** after live re-check; Task-221 cancelled.
- `V-4` Early probe (zero MCP permission frames) is treated as environment/version-stale relative to current desktop Grok + Task-218 config; product acceptance is the desktop path.

### Constraints

- Do not weaken Claude/Codex/Gemini MCP gating.
- Do not create pending state the interactive desktop cannot resolve (no hang).
- No new code for this close-out.

### Open Questions

- `Q-1` **SUPERSEDED (2026-07-11):** desktop shows MCP Drive **does** reach the permission channel under YOLO=off (card for `google-drive__authGetStatus`). Earlier probe answer no longer drives product action.
- `Q-2` MCP write under YOLO=off — still optional spot-check; not blocking close-out after read-path (authGetStatus) full gate verification.
- `Q-3` PreToolUse — **won't-do** (Task-221 cancelled).

### Source Refs

- `grok_adapter.go` `handleInbound` (Task-208), `grok_process.go` permission_mode rewrite (Task-218), Grok `config.toml` `[mcp_servers.google-drive]`.
- Live acceptance 2026-07-11 (user): same Drive-email prompt — ApprovalCard; deny blocks; YOLO on auto; 2–3× stable.

## 1. Issue Summary

Originally: under YOLO=off, Grok appeared to run external MCP (Google Drive) without approval while native write/exec gated correctly.  
Closed: desktop Grok + YOLO=off shows standard ApprovalCard for Drive MCP; deny/approve and YOLO=on behavior verified.

## 2. Parent Links

- impacted coding plan: `CP-46` (Grok controlled adapter over ACP)
- impacted tech design: `SD-09` (Approval Gates)
- impacted system spec: `SS-08` (YOLO is the SSOT; YOLO=off must gate)

## 3. Environment and Reproduction

- environment: FlowPilot desktop + local runner, Grok provider, YOLO toggle off, Google Drive MCP configured for the account (`~/.grok/config.toml` `[mcp_servers.google-drive]`, `permission_mode=default`).
- original repro steps: with YOLO off, send `Using mcp google drive to let me know the current google email of this drive`.
- frequency (original report): deterministic auto-run. **Post-fix:** deterministic gate.

## 4. Expected vs Actual

- expected: an approval card appears and the MCP tool runs only after approval.
- actual (2026-07-10 report): Grok executes MCP immediately, no card.
- actual (2026-07-11 close): card appears (`google-drive__authGetStatus`); approve runs; deny blocks.

## 5. Impact

- users affected: Grok users relying on YOLO=off to review MCP/Drive actions — **gate works** under current stack.
- severity at open: medium-high; at close: **resolved for verified Drive path**.

## 6. Root Cause

### As diagnosed 2026-07-10 (historical)

- Code trace + scripted probe: Grok did not emit `session/request_permission` for some MCP/`CallMcpTool` paths; runner never saw MCP for gating. Proxy chat path cannot surface ApprovalCard (`hasNoApprovalScope`).

### As understood at close 2026-07-11

- With Task-208 + Task-218 + Drive wired in Grok `config.toml`, desktop Grok **does** emit permission requests for Drive tools into `handleInbound` → ApprovalCard.
- Proxy remains a non-UI gate for chat; it is not the mechanism that fixed the UX.
- Task-209 prompt reinforcement is unrelated (ask_user/spawn only).

## 7. Fix Strategy

- `F-1` **Done via existing work:** Task-208 permission channel + Task-218 YOLO enforce; Grok emits MCP permission requests under current conditions — no new interceptor.
- `F-2` Explicitly NOT Google Drive proxy scope wiring for chat (still rejected, `V-2`).
- `F-3` Task-221 PreToolUse: **won't-do / cancelled**.

## 8. Validation

- `V-4` **PASS (user 2026-07-11):** YOLO=off Drive MCP → ApprovalCard; deny blocks; approve runs.
- `V-5` **PASS (user 2026-07-11):** YOLO=true auto-runs MCP, no card.
- `V-6` No code change this close-out → Claude/Codex/Gemini gating unchanged by this ticket.
- Repeat stability: **PASS** (2–3× same prompt).

## 9. Regression Guard

- tests: existing Task-208 unit/concurrent permission tests remain the automated guard for the channel; no new PreToolUse suite.
- audit checks: Grok native-tool gating (Task-208) and YOLO posture (Task-218) stay green.
- reopen trigger: Drive (or other MCP) under YOLO=off auto-runs with zero ApprovalCard again.

## 10. Follow-Up Document Updates

- [Task-221](../../08-Task/done/Task-221-Grok-MCP-Tool-Gating-Via-PreToolUse-Hook.md) → `cancelled` (won't-do PreToolUse), moved to `done/`.
- Task-209 remains separate (MCP wiring / ask_user / spawn).
- notes left unchanged on purpose: Google Drive proxy admin-web approval flow unchanged.
