# CA-283: Close BUG-273 / Task-221 MCP Gate As Won't-Do PreToolUse

## Scope

Docs close-out only: Grok Google Drive MCP under YOLO=off already surfaces the standard ApprovalCard via Task-208 + Task-218. Cancel Task-221 PreToolUse plan; mark BUG-273 done; update CP-46 Q-3/R-4/DOD verification.

## Root Cause (why no new code)

- Early ACP probe (2026-07-10) suggested Grok never emitted `session/request_permission` for MCP/`CallMcpTool`.
- Desktop re-check (2026-07-11) showed Drive `authGetStatus` **does** gate through Task-208 `handleInbound` when Task-218 keeps `permission_mode=default` under YOLO=false.
- PreToolUse hook would duplicate an already-working path.

## Changes

- `requirements/08-Task/done/Task-221-…` — status `cancelled` (won't-do PreToolUse); product DOD-P1..P5 closed by live verify.
- `requirements/09-BugFix/done/BUG-273-…` — status `done` (fixed-by-existing-path).
- Removed inprogress copies of both docs.
- `requirements/07-Coding-Plan/inprogress/CP-46-…` — Q-3 answered, R-4 mitigated, §10/§10.1/§10.2 MCP YOLO gate marked done, child links updated.
- `Task-209` follow-up note: Task-221 cancelled for MCP gating.
- `CA-281` verification: live DOD-8/9 confirmed 2026-07-11 (pre-existing working-tree note).

## Verification

- User desktop 2026-07-11:
  1. YOLO=off deny → Drive tool blocked / no email
  2. YOLO=on → no card, Drive auto
  3. Same prompt 2–3× stable
- No production code in this change set.

## Follow-ups

- Reopen hard-gate task only if MCP stops emitting `session/request_permission` under YOLO=off.
- Optional: fix Grok `extraMCPServers` to not depend on `.claude.json` under `GROK_HOME` (Drive today uses ambient `config.toml`).

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-221
change_type: docs
summary: Close BUG-273 and cancel Task-221 PreToolUse — Grok Drive MCP YOLO gate already met via Task-208/218; update CP-46 DOD
# --->8---
