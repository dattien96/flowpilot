# BUG-124: Codex Spawn-Agent Reserved Tool Name

## Metadata

- Document ID: `BUG-124`
- Title: `Codex Spawn-Agent Reserved Tool Name`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `none`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [CA-085: Fix Codex Ask-User Dynamic Tool And Nudge](../../../change-audit/CA-085-fix-codex-ask-user-dynamic-tool-and-nudge.md)
- Replaces: `none`
- Tags: `codex, spawn-agent, dynamic-tools, runner, regression`

## AI Quick View

### Summary

- Codex turns that exposed the `spawn_agent` dynamic tool began failing with an OpenAI `invalid_request_error` before the model could act.
- The failure message reported `Function 'functions.spawn_agent' is reserved for encrypted tool use by this model`, which indicates a name collision inside the Codex/OpenAI tool surface.
- Renaming the current registration was insufficient for existing chats because Codex reloads the original dynamic-tool declarations from rollout `session_meta` during `thread/resume`.
- Claude was not affected because it exposes `spawn_agent` through the FlowPilot MCP server, not Codex dynamic tools.
- Fix: advertise a Codex-only FlowPilot alias, migrate legacy rollout metadata before resume, and normalize the alias back to `spawn_agent` inside the runner/UI boundary.

### Current Ask

- Completed: Codex can expose and call the spawn-agent tool again without hitting the reserved tool-name rejection.

### Key Decisions

- `V-1` Rename only the Codex dynamic tool registration name to `flowpilot_spawn_agent`; keep Claude MCP `spawn_agent` unchanged.
- `V-2` Accept both `spawn_agent` and `flowpilot_spawn_agent` in the Codex inbound tool-call router for backward compatibility.
- `V-3` Normalize the alias back to `spawn_agent` in mapped provider events so timeline/history behavior stays unchanged.
- `V-4` Before resuming a legacy Codex rollout, atomically replace only `dynamic_tools[].name = "spawn_agent"` in its first `session_meta` record and preserve all later records.

### Constraints

- Do not change the spawn-agent input schema or orchestration semantics.
- Keep the fix Codex-specific to avoid unnecessary surface changes on Claude.
- Preserve existing UI labels and runner transcript behavior.

### Open Questions

- None.

### Source Refs

- User-reported error on `2026-06-22`: `Invalid Value: 'tools'. Function 'functions.spawn_agent' is reserved for encrypted tool use by this model and must match the configured declaration.`
- `apps/local-runner/internal/runner/codex_adapter.go`
- `apps/local-runner/internal/runner/codex_event_mapper.go`
- `apps/local-runner/internal/runner/codex_appserver_test.go`

## 1. Issue Summary

On `2026-06-22`, asking Codex about started sub-agents triggered a turn failure before model execution. The app-server surfaced an OpenAI `400 invalid_request_error` tied to the `tools` parameter and the reserved function name `functions.spawn_agent`.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: Desktop FlowPilot local runner using the Codex provider and shared app-server dynamic tools.
- reproduction steps:
  1. Open a Codex-backed chat.
  2. Send a prompt that can cause the model to inspect or use sub-agent state.
  3. The turn fails with a `400 invalid_request_error` mentioning reserved function `functions.spawn_agent`.
- frequency: reproducible for new turns before the alias fix and for every resumed chat whose rollout metadata still contains the legacy name.

## 4. Expected vs Actual

- expected: Codex accepts the FlowPilot spawn-agent tool declaration and can call it during the turn.
- actual: the turn is rejected during tool declaration validation because `spawn_agent` collides with a reserved encrypted-tool function name.

## 5. Impact

- users affected: Codex users on chats that expose the FlowPilot spawn-agent tool.
- workflows affected: Codex multi-agent chat turns, especially threads that need to inspect or spawn sub-agents.
- severity: High, because the turn fails before model execution.

## 6. Root Cause

- hypothesis: newer Codex/OpenAI tool validation reserves the derived function name for `spawn_agent`, even though the runner-side dynamic tool plumbing remains correct.
- confirmed cause: FlowPilot advertised Codex dynamic tool name `spawn_agent`; the live backend rejected the resulting function name `functions.spawn_agent` as reserved. Existing rollouts persisted that declaration in `session_meta.dynamic_tools`, and Codex reloaded it during `thread/resume`, bypassing the corrected resume payload.
- evidence: the reported error explicitly names `functions.spawn_agent`; the failed rollout contains `dynamic_tools[].name = "spawn_agent"` in its first record; a fresh real Codex thread succeeds with the alias while the legacy resumed thread continues failing until its metadata is migrated.

## 7. Fix Strategy

- `F-1` Change the Codex dynamic tool registration name from `spawn_agent` to `flowpilot_spawn_agent`.
- `F-2` Keep `handleDynamicToolCall` backward-compatible by accepting both names.
- `F-3` Normalize the alias back to `spawn_agent` when mapping Codex tool events so UI/history output does not change.
- `F-4` Add regression tests for alias registration, alias invocation, and event-name normalization.
- `F-5` Bind the active `CODEX_HOME` to the Codex adapter and migrate legacy rollout `session_meta` before invoking `thread/resume`.
- `F-6` Perform the migration through a sibling temporary file plus rename, preserving file permissions and every rollout record after the first line.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'TestCodexAdapter(AskUserDynamicToolRoundTrip|SpawnAgentDynamicToolAliasRoundTrip|ResumedTurnRoutesAskUserDynamicTool)|TestMapCodexNotification' -count=1` — pass.
- `V-2` `go test ./internal/runner -count=1` — pass.
- `V-3` `git diff --check` — pass.
- `V-4` `TestCodexAdapterResumeMigratesLegacySpawnAgentTool` — pass; confirms migration occurs before `thread/resume`, preserves later rollout records, and preserves file mode.
- `V-5` `FLOWPILOT_CODEX_E2E=1 go test ./internal/runner -run TestCodexAskUserEndToEnd -count=1 -v` — pass against the real Codex app-server with the aliased tool declaration present.
- `V-6` Replayed the original prompt against legacy `run-190` after restarting the rebuilt runner — rollout metadata changed to `flowpilot_spawn_agent` and the real resumed turn completed without the reserved-tool `400`.

## 9. Regression Guard

- tests: `TestCodexAdapterSpawnAgentDynamicToolAliasRoundTrip`, `TestCodexAdapterResumeMigratesLegacySpawnAgentTool`, resumed-turn dynamic-tools assertions, and Codex event-mapper alias normalization coverage.
- alerts: any future `invalid_request_error` referencing `functions.spawn_agent` on Codex turns indicates the alias regressed or was bypassed.
- audit checks: Codex thread-start/thread-resume payloads and resumed rollout `session_meta` must advertise `flowpilot_spawn_agent`, not raw `spawn_agent`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none required; this is a provider-compatibility delta, not a behavior redesign.
- notes left unchanged on purpose: Claude MCP continues to expose `spawn_agent` because that surface is not subject to this Codex-specific collision.
