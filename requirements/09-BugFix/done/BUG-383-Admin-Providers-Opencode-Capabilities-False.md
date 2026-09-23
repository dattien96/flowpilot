# BUG-383: `/admin/providers` misreports opencode capabilities (`approvalEvents:false, mcp:false`) — evaluated on zero-value adapter before wiring

## Metadata

- Document ID: `BUG-383`
- Title: `Capabilities() computed on &opencodeAdapter{} at registration → mcpServer-dependent flags permanently false while both features are live`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-57-Opencode-Provider-Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md), [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md)
- Feature Keys: `ai-providers`

## AI Quick View

### Summary

- `GET /admin/providers` reports opencode `{"approvalEvents":false,"mcp":false}` while both are provably live: `permission_required` approvals emitted and decisions applied (appr-35 deny, appr-55/66/73 approve), `user_question_required`/`agent_result_injected` via the FlowPilot MCP tools work, and every `session/load`/`session/new` carries `mcpServers:[{name:flowpilot,type:http,…}]`.
- Root cause: `provider_registry.go:577` registers `Capabilities: (&opencodeAdapter{}).Capabilities()` — evaluated on a **zero-value adapter**. `opencodeAdapter.Capabilities()` (`opencode_adapter.go:195-203`) derives `ApprovalEvents`/`Mcp` from `a.mcpServer != nil`, but `mcpServer` is only assigned later inside `newAdapterForTurn` (`provider_registry.go:623` `a.mcpServer = r.claudeMCP`) → the registered snapshot is permanently false.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Triage note: audit whether other flags/providers are derived from post-registration wiring the same way (codex/claude read true because their registration-time adapters differ — verify).

## Bug report

- **Symptom**: Admin/provider metadata says opencode cannot do approval events or MCP.
- **Expected**: `approvalEvents:true, mcp:true` — both features work live.
- **Actual**: `l57-1_admin_providers.json` — `{"streaming":true,"resume":true,"approvalEvents":false,"fileEvents":true,"skillSelection":true,"mcp":false,"interrupt":true,"vision":false}`.
- **Impact**: low-medium — metadata lie; any client (TUI/Desktop) gating approval panels or MCP affordances on these flags would hide working opencode functionality. Runtime itself is unaffected.

## Reproduction

1. Runner :19257 with opencode installed (1.18.30). `GET /admin/providers` → opencode entry has `approvalEvents:false, mcp:false`.
2. `yoloMode:false` chat turn requesting a file write → `permission_required` appr-35 IS emitted; decision `deny` applied via `POST /client/approvals/{id}/decision`.
3. Ask the model to use FlowPilot MCP `ask_user` → `user_question_required` q-103 emitted and answered; `spawn_agent` wait=true → child run-121, `agent_result_injected "child-ok"`.
4. `runner.log` — every `session/load`/`session/new` carries `mcpServers:[{name:flowpilot,type:http,url:…/internal/claude-permission-mcp?token=…}]`.

## Root cause

- `apps/local-runner/internal/runner/provider_registry.go:577` — `Capabilities: (&opencodeAdapter{}).Capabilities()` computed at registration on a zero-value adapter.
- `apps/local-runner/internal/runner/opencode_adapter.go:195-203` — `Capabilities()` derives `ApprovalEvents`/`Mcp` from `a.mcpServer != nil`.
- `provider_registry.go:623` — `a.mcpServer = r.claudeMCP` wired only inside `newAdapterForTurn`, after registration → the snapshot stays false forever.

## Evidence

- `~/fp-beds/lt-evidence/cp57/BUG-LIVE-57-2.md`, `RESULT.md` (L-57-1 caveat, L-57-4/L-57-5 proofs), `l57-1_admin_providers.json` (flags), `l57_run29_events_post_restart.json` seq 11-15/17-22 (permission_required ×4, user_question_required, agent_result_injected), `runner.log` session/load `mcpServers` lines.

## Severity

- low

## Completion Notes (implemented 2026-09-23, CA-917)

- Fix: provider registrations advertise static capability literals — opencode
  and devin no longer evaluate `Capabilities()` on zero-value adapters
  (mcpServer is wired per turn). Grok's literal also gained `Mcp: true`
  (same surface defect).
- Files: `provider_registry.go`.
- Tests: `bug383_admin_capabilities_test.go` asserts approvalEvents+mcp true
  for both registrations.
