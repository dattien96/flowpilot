# BUG-105: Claude Provider spawn_agent Unavailable Due To Silent MCP Degradation

## Metadata

- Document ID: `BUG-105`
- Title: `Claude Provider spawn_agent Unavailable Due To Silent MCP Degradation`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-104](BUG-104-Agents-Panel-Spawn-Button-Badge-And-Description-Regressions.md)
- Replaces: `None`
- Tags: `multi-agent, claude, mcp, spawn-agent, silent-failure`

## AI Quick View

### Summary

- The same `spawn_agent` prompt sent to a Codex session works but fails on a Claude session: Claude replies "I don't have a tool called `spawn_agent`" and lists only its native tools.
- Root cause: two silent-degradation paths in `claude_adapter.go/SendTurn` suppress FlowPilot MCP tool availability without any log or error. Path 1 — `mcpBaseURL` is empty + YOLO=true → prompt delivered without `--mcp-config`. Path 2 — `waitReady` times out (MCP connection not established) → return value was silently ignored and prompt delivered anyway.
- In both paths Claude's `tools/list` only shows native Claude Code tools (Agent, Bash, Edit, …) and `mcp__flowpilot__spawn_agent` is absent; no FlowPilot spawn_agent, ask_user, or approve tools are visible.
- No test guarded that `spawn_agent` was present in `claudeMCPToolDefs()` (only `ask_user` had a schema test).

### Current Ask

- Log a warning when `mcpBaseURL` is empty and YOLO=true so operators can detect the misconfiguration rather than silently losing FlowPilot MCP tools.
- Log a warning when `waitReady` returns false (MCP connection timed out) before the prompt is delivered.
- Add `TestClaudeSpawnAgentToolAdvertisesSchema` to `claude_permission_mcp_test.go` as a regression guard parallel to the existing `ask_user` test.

### Key Decisions

- `V-1` Cross-provider spawning (`provider="codex"` from a Claude parent) is NOT the root cause and is a supported feature — `spawnChildRun` correctly resolves the provider from `in.Provider` → `agentDef.Provider` → parent's `providerKey`. No server-side mismatch error is added.
- `V-2` Silent degradation itself is not removed (YOLO=true can operate without MCP; gated turns already fail-closed). The fix only adds visibility via log lines so operators can detect the condition.

### Constraints

- Both degradation paths are intentional design choices (YOLO resilience, fail-closed for gated turns). The log lines are warnings only — they do not change behavior or throw errors.
- The `sh` test helper fails on Windows for tests that exercise the full YOLO degradation path (`TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade/yolo=on`); that failure is pre-existing and unrelated to this fix.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/claude_adapter.go`
- `apps/local-runner/internal/runner/claude_permission_mcp_test.go`
- `apps/local-runner/internal/runner/claude_mcp_server.go` — `claudeMCPToolDefs()` confirmed to include `spawn_agent` with correct schema

## 1. Issue Summary

The same test prompt (`Use spawn_agent exactly once with agent="reviewer", provider="codex", wait=true. Child prompt: "Do not use tools. Reply exactly: CHILD_AGENT_DONE." After the child returns, tell me the exact child result.`) works when sent to a Codex session but consistently fails on a Claude session.

Claude responds: `"I don't have a tool called spawn_agent — it doesn't exist in my available toolset. My tools are: Agent, Bash, Edit, Glob, Grep, PowerShell, Read, ScheduleWakeup, ShareOnboardingGuide, Skill, ToolSearch, Workflow, and Write."`

The listed tools are Claude's native tools only — none of the `mcp__flowpilot__*` tools appear, indicating the FlowPilot per-turn MCP server was not connected when the prompt was delivered.

For Codex the difference is structural: `spawn_agent` is registered as a DynamicTool directly in the process (in-process, no HTTP server). For Claude it is served via the runner-hosted HTTP MCP server and requires a successful per-turn connection.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- system spec: Task-082 is the originating task for `spawn_agent`; no separate SS covers MCP connectivity

## 3. Environment and Reproduction

- environment: Desktop app, Claude provider, any platform
- reproduction steps:
  1. Open a Claude session in FlowPilot.
  2. Send: `Use spawn_agent exactly once with agent="reviewer", prompt="CHILD_AGENT_DONE", wait=true.`
  3. Observe Claude replies "I don't have a tool called spawn_agent" and lists only native tools.
  4. Send the identical prompt to a Codex session — it succeeds.
- frequency: Consistent (reproducible on first turn of a Claude session when MCP connection is absent)

## 4. Expected vs Actual

- expected:
  - Claude's `tools/list` includes `mcp__flowpilot__spawn_agent`, `mcp__flowpilot__ask_user`, `mcp__flowpilot__approve` alongside native tools.
  - Claude can call `spawn_agent` to spawn a Codex child run, even with `provider="codex"` (cross-provider spawning is supported).
- actual:
  - Claude's `tools/list` only contains native Claude Code tools.
  - Claude reports `spawn_agent` as unknown and declines to act.

## 5. Impact

- users affected: All Claude provider users attempting to use `spawn_agent` (child agent spawning from Claude sessions)
- workflows affected: Multi-agent spawning via Claude parent — Task-082 core feature
- severity: High — spawn_agent from Claude is the primary mechanism for Claude-orchestrated multi-agent workflows; the feature is entirely broken for Claude when MCP is unavailable

## 6. Root Cause

- hypothesis: Two silent-degradation paths in `SendTurn` suppress FlowPilot MCP availability with no log output.
- confirmed cause:
  1. **Path A** (`claude_adapter.go` line ~108): when `a.mcpBaseURL()` returns `""` and `posture.RunnerAutoApprove` is true, the block exits with a comment about `ask_user` in-stream fallback — but `spawn_agent` has no fallback. No log was emitted.
  2. **Path B** (`claude_adapter.go` line ~165): `a.mcpServer.waitReady(turnCtx, mcpToken, a.mcpReadyTimeout)` returned a `bool` indicating timeout, but the return value was discarded. When Claude's MCP client doesn't connect within the timeout, the prompt is delivered silently without FlowPilot tools.
  3. **Missing test**: `claude_permission_mcp_test.go` had `TestClaudeAskUserToolAdvertisesSchema` but no equivalent for `spawn_agent`, so a regression where `spawn_agent` was removed from `claudeMCPToolDefs()` would go undetected.
- evidence: Code inspection of `claude_adapter.go:SendTurn`; `claudeMCPToolDefs()` in `claude_mcp_server.go` confirmed to include `spawn_agent`. The user's error response listed ONLY native Claude Code tools — zero `mcp__flowpilot__*` tools — proving the MCP connection was absent when the prompt was delivered.

## 7. Fix Strategy

- `F-1` Add `"log"` import to `claude_adapter.go`.
- `F-2` In the `base == ""` + YOLO=true branch, emit: `log.Printf("[claude-mcp] MCP base URL not set; FlowPilot tools (spawn_agent, ask_user) are unavailable for this turn — check runner startup")`.
- `F-3` Capture the `waitReady` return value; when false emit: `log.Printf("[claude-mcp] MCP connection did not become ready within timeout; FlowPilot tools (spawn_agent, ask_user) may be missing from this turn — the prompt will be delivered anyway")`.
- `F-4` Add `TestClaudeSpawnAgentToolAdvertisesSchema` in `claude_permission_mcp_test.go` verifying `spawn_agent` is in `claudeMCPToolDefs()` with `agent`, `prompt`, `provider`, and `wait` properties and `required` contains at least `agent` + `prompt`.

## 8. Validation

- `V-1` `TestClaudeSpawnAgentToolAdvertisesSchema` passes (new test).
- `V-2` `TestClaudeAskUserToolAdvertisesSchema`, `TestClaudeArgsIncludesStrictMcpConfig`, `TestClaudeArgsDisablesBuiltinAskUserQuestion`, `TestClaudeMCPServerPromptGate` all continue to pass.
- `V-3` `TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade/yolo=on` now logs the warning before spawning (visible in test output: `[claude-mcp] MCP base URL not set…`).
- `V-4` The `yolo=on` sub-test itself fails due to pre-existing `sh` not found on Windows — confirmed unrelated (same failure exists without this change).

## 9. Regression Guard

- tests: `TestClaudeSpawnAgentToolAdvertisesSchema` guards that `spawn_agent` stays in `claudeMCPToolDefs()` with required fields. `TestClaudeMCPServerPromptGate` guards that `waitReady` properly returns false on timeout.
- alerts: Runner logs `[claude-mcp]` prefix lines when either degradation path fires — operators can grep for these.
- audit checks: `claude_mcp_server.go/claudeMCPToolDefs()` is the SSOT for Claude's MCP tool list; both `ask_user` and `spawn_agent` now have schema tests.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — the behavior of `claudeMCPToolDefs()` and MCP wiring is unchanged; this fix only surfaces a previously invisible failure mode.
- notes left unchanged on purpose: Cross-provider spawning (`provider="codex"` from a Claude parent) is supported and working; no mismatch error was added. The `spawn_agent` MCP tool definition itself was already correct.
