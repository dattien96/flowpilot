# Task-221: Grok MCP Tool Gating Via PreToolUse Hook

## Metadata

- Document ID: `Task-221`
- Title: `Grok MCP Tool Gating Via PreToolUse Hook`
- Phase: `task`
- Status: `cancelled` — **won't-do PreToolUse**; product acceptance for BUG-273 already met by Task-208 + Task-218 (user live-verified 2026-07-11). No hook implementation.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-11`
- Parent Documents: [BUG-273: Grok Does Not Gate MCP Tool Calls Under YOLO=Off In Chat](../../09-BugFix/done/BUG-273-Grok-Does-Not-Gate-MCP-Tool-Calls-Under-Yolo-Off-In-Chat.md), [Task-208: Grok Permission Channel And YOLO Posture](./Task-208-Grok-Permission-Channel-And-Yolo-Posture.md), [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Child Documents: `None`
- Related Documents: [Task-209: Grok MCP Ask-User Spawn-Agent Parity](./Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md) (wires MCP tools; prompt reinforcement is **not** the Drive gate), [Task-218: Grok YOLO Enforced Via Config Rewrite And Always-Approve Flag](./Task-218-Grok-Yolo-Enforced-Via-Config-Rewrite-And-Always-Approve-Flag.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- Replaces: `None`
- Tags: `grok, mcp, yolo-policy, approval, hook, acp, ai-providers, cancelled, wont-do`

## AI Quick View

### Summary

- Original plan: build a FlowPilot `PreToolUse` hook so Grok MCP tools (`CallMcpTool` / Drive) reach `RequestApproval` under YOLO=off, after an early probe suggested Grok never emits `session/request_permission` for MCP.
- **Outcome (2026-07-11): cancelled / won't-do.** Desktop live re-check (user) shows Grok **does** gate Google Drive MCP via the existing Task-208 channel once Task-218 keeps `permission_mode=default` under YOLO=off. PreToolUse is unnecessary complexity for the current product goal.
- Working path: YOLO=off → Grok emits `session/request_permission` for Drive tools (e.g. `google-drive__authGetStatus`) → `grokAdapter.handleInbound` → `TurnBridge.RequestApproval` → standard ApprovalCard; YOLO=on auto-approves with no card.

### Current Ask

- None — closed. Reopen only if Grok stops emitting MCP permission requests (hard guarantee / non-Drive MCP regression).

### Key Decisions

- `T-1` Gate through the EXISTING `TurnBridge.RequestApproval` → standard approval card (resolved via `SubmitApprovalDecision`), NOT the Google Drive proxy's file-based approval store — the interactive desktop has no UI for the latter and a proxy pending-record would hang the turn (BUG-273 `V-1`/`V-2`). **Still valid; already satisfied by Task-208.**
- `T-2` ~~Mechanism: PreToolUse hook~~ **Superseded:** Grok's own `session/request_permission` for MCP is sufficient under current binary + Task-218 posture.
- `T-3` YOLO=true fast-path already handled in `handleInbound` / `--always-approve` (Task-208/218) — no hook required.
- `T-4` Hook install discipline **N/A** (hook not built).
- `T-5` (2026-07-11) **Won't-do PreToolUse.** Product acceptance closed via user live verification against Drive MCP; no new unit/live hook tests.

### Constraints

- **ZERO BASE REGRESSION (CP-46 `P-0`):** no code change in this close-out; Claude/Codex/Gemini untouched.
- Must not hang a turn — existing bridge path already fails safe on expiry/interrupt (deny).
- Must not reintroduce ambient MCP/compat scanning disabled by Task-206.
- YOLO=true behavior (auto-run) must not regress — live-confirmed.

### Open Questions

- `Q-1`–`Q-4` (PreToolUse contract / blockability / read-class coverage / proxy fallback): **moot** for this close-out. Revisit only if MCP permission emission regresses.

### Source Refs

- `BUG-273` (closed with this decision), `Task-208` (permission channel), `Task-218` (YOLO config rewrite / `--always-approve`).
- `apps/local-runner/internal/runner/grok_adapter.go` (`handleInbound` → `RequestApproval`), `grok_process.go` (`permission_mode` rewrite under YOLO=false), `google_drive_mcp_provider_config.go` (Grok Drive `config.toml` wiring).
- Live acceptance 2026-07-11 (user desktop): Drive `authGetStatus` under YOLO=off shows ApprovalCard; deny blocks / no email; YOLO=on no card + auto-run; stable across 2–3 repeats of the same prompt.

## 1. Goal

Under YOLO=off, Grok external MCP tool calls (Google Drive, and any MCP server that Grok routes through `session/request_permission`) surface the standard FlowPilot approval card and honor deny/approve — without a PreToolUse hook, hanging turns, or touching other providers.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-09`
- system spec: `SS-08`
- specific upstream ids: `BUG-273`, `Task-208 T-1..T-4`, `Task-218`

## 3. Trigger

BUG-273 initially claimed Grok never asks permission for MCP tool calls. A later desktop re-check (after Task-208/218 + Drive wiring for Grok) showed the gap no longer holds for Google Drive under YOLO=off.

## 4. Exact Change

- **None (won't-do PreToolUse).** No hook install, no hook HTTP callback, no new adapter path.
- Document-only close-out: mark cancelled, record that acceptance is met by Task-208 + Task-218, close BUG-273 accordingly.

## 5. Touched Areas

- files: this task doc + BUG-273 (moved to `done/`); **no production code**.
- modules: none.
- routes: none.
- tables: none.

## 6. Acceptance Check

- YOLO=off: Grok Google Drive tool call surfaces the standard approval card; deny prevents execution; approve executes. **PASS (user, 2026-07-11).**
- YOLO=true: MCP tools auto-run, no card. **PASS (user, 2026-07-11).**
- Native write/exec gating (Task-208) and Claude/Codex/Gemini gating unchanged. **No code change this close-out.**
- No turn hangs; fail-safe deny on bridge error already in Task-208 path.

### 6.1 Definition of Done (DOD)

PreToolUse-specific DODs are **N/A (won't-do)**. Product acceptance mapped instead:

- [x] `DOD-P1` YOLO=off Drive MCP → ApprovalCard; approve runs. *(user live)*
- [x] `DOD-P2` YOLO=off Drive MCP → deny blocks (no email / no successful tool result). *(user live)*
- [x] `DOD-P3` YOLO=true Drive MCP → no card, auto-run. *(user live)*
- [x] `DOD-P4` Stable across 2–3 repeats of the same prompt. *(user live)*
- [x] `DOD-P5` No PreToolUse code introduced; no global hook config mutation.
- [~] Original `DOD-1`–`DOD-5` (hook contract/install/tests): **cancelled — not implemented by design.**

## 7. Out of Scope

- Implementing PreToolUse / plugin-dir hooks (won't-do).
- Gating read-class **native** tools beyond what Grok already prompts.
- The Google Drive proxy's admin-web approval flow (unchanged).
- MCP tool *wiring* — Task-209 (prompt reinforcement for ask_user/spawn is separate and is **not** the Drive approval mechanism).

## 8. Completion Notes

- result: **cancelled / won't-do PreToolUse.** Product goal of BUG-273 is satisfied by existing Task-208 permission channel + Task-218 YOLO posture + Grok Drive MCP in `config.toml`.
- implementation notes: No new code. Working mechanism is `session/request_permission` → `handleInbound` → `RequestApproval` → ApprovalCard. Prompt updates from Task-209 do **not** create Drive approval cards.
- verification: User desktop 2026-07-11 — (1) deny blocks Drive tool / no email, (2) YOLO on → no card + auto, (3) same prompt repeated 2–3× OK.
- follow-ups: Reopen a hard-gate task (hook or proxy-bridge) only if Grok stops emitting MCP `session/request_permission` or non-Drive MCP fails the same checks. Optional cleanup: Grok `extraMCPServers` currently reuses `flowpilotClaudeExtraMCPServers` (reads `.claude.json` under `GROK_HOME`) — Drive today relies on ambient `config.toml`; not blocking this close-out.
- upstream docs updated: BUG-273 → `done` / fixed-by-existing-path; this task → `done/` as cancelled.
