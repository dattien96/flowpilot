# BUG-114: Claude spawn_agent Missing When Co-Resident MCP Server Is Slow

## Metadata

- Document ID: `BUG-114`
- Title: `Claude spawn_agent Missing When Co-Resident MCP Server Is Slow`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-105: Claude Provider Spawn Agent MCP Silent Degradation](./BUG-105-Claude-Provider-Spawn-Agent-MCP-Silent-Degradation.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `claude, mcp, spawn-agent, google-drive, timeout, multi-agent`

## AI Quick View

### Summary

- In a Claude parent session, asking to spawn a (Codex) sub-agent failed with the model replying "I don't have a spawn_agent tool." It had worked previously.
- Root cause: the FlowPilot MCP tools (`spawn_agent`, `ask_user`) are only offered to a Claude turn once the per-turn MCP client finishes connecting within a 10s window. When Google Drive MCP is configured for Claude, the runner injects it as a `command`/stdio sidecar into the same per-turn `--mcp-config`. The Claude CLI launches that sidecar during MCP init; a slow warm-up (npx/OAuth/proxy) stalls init past 10s, so FlowPilot's `tools/list` arrives too late, `waitReady` times out, and the prompt is sent WITHOUT the FlowPilot tools.
- Fix: raise the MCP-ready ceiling from 10s to 30s. `waitReady` returns the instant FlowPilot's tools connect, so this adds no latency to healthy turns — it only grants grace when a co-resident server is slow.
- Confirmed fixed by runner log: a Claude parent (`run-1`) spawned a Codex child (`run-14`) successfully after the change.

### Current Ask

- Restore reliable `spawn_agent`/`ask_user` availability for Claude turns that also load a co-resident MCP server.

### Key Decisions

- `V-1` Raise `claudeMCPReadyDefaultTimeout` 10s → 30s. This is the documented fragility from [BUG-105](./BUG-105-Claude-Provider-Spawn-Agent-MCP-Silent-Degradation.md) (silent degradation when the MCP client isn't ready); BUG-105 added logging, this widens the window.
- `V-2` No change to tool registration (`claudeMCPToolDefs` already advertises `spawn_agent` unconditionally) or to the MCP base-URL wiring (`SetMCPBaseURL` at startup) — both verified intact.

### Constraints

- Tests set their own short `mcpReadyTimeout`, so the default change does not affect them.
- The deeper isolation (giving FlowPilot's required tools a connection path independent of optional sidecars) is deferred; the timeout widening resolves the observed slow-warmup case.

### Open Questions

- If a co-resident MCP server hangs indefinitely (never connects), 30s still degrades to sending without tools. A follow-up could isolate required vs optional MCP servers. Not observed in practice after the fix.

### Source Refs

- `apps/local-runner/internal/runner/claude_mcp_server.go` — `claudeMCPReadyDefaultTimeout`
- `apps/local-runner/internal/runner/claude_adapter.go` — `waitReady` gate before delivering the prompt
- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go` — `flowpilotClaudeExtraMCPServers` (the injected sidecar)

## 1. Issue Summary

A Claude parent run could not spawn a sub-agent: the model said it had no `spawn_agent` tool. The behavior was intermittent and correlated with having Google Drive MCP configured for the Claude account.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- related bugfix: [BUG-105](./BUG-105-Claude-Provider-Spawn-Agent-MCP-Silent-Degradation.md)

## 3. Environment and Reproduction

- environment: Desktop + local runner, Claude provider account with Google Drive MCP configured, Windows.
- reproduction steps:
  1. Start a chat on the Claude provider.
  2. Ask Claude to spawn a sub-agent (e.g. a Codex reviewer).
  3. Observe Claude reply that no `spawn_agent` tool exists.
- frequency: When the co-resident MCP sidecar is slow to warm up within the 10s window.

## 4. Expected vs Actual

- expected: Claude turns always have `spawn_agent`/`ask_user`.
- actual: When MCP init stalled past 10s, the prompt was delivered without the FlowPilot tools.

## 5. Impact

- users affected: Claude-provider users with a co-resident MCP server (e.g. Google Drive) configured.
- workflows affected: Spawning sub-agents and structured `ask_user` from Claude turns.
- severity: High — a core multi-agent capability silently unavailable.

## 6. Root Cause

- confirmed cause: The Claude adapter withholds the prompt until the per-turn MCP client connects (`waitReady`, bounded by `claudeMCPReadyDefaultTimeout = 10s`). Google Drive MCP is merged into the same per-turn `--mcp-config` as a stdio sidecar (`flowpilotClaudeExtraMCPServers`). The Claude CLI launches it during MCP init; a slow warm-up pushes FlowPilot's `tools/list` past 10s, so `waitReady` times out and the turn is sent without FlowPilot tools — the model then reports no `spawn_agent`.
- evidence:
  - Tool registration (`claudeMCPToolDefs`) and base-URL wiring verified unchanged; only the runtime connection timing gates the tools.
  - Runner log after the fix: `[agent-spawn] child created parent="run-1" child="run-14" agent="reviewer" provider="codex"` — a Claude parent successfully spawned a Codex child.

## 7. Fix Strategy

- `F-1` Raise `claudeMCPReadyDefaultTimeout` from 10s to 30s. Because `waitReady` returns the moment FlowPilot's tools connect, healthy turns are unaffected; only a slow co-resident sidecar gets extra grace before degradation.

## 8. Validation

- `V-1` `go build` and `go vet` clean.
- `V-2` 25 Claude/MCP runner tests pass (they use their own short timeouts, unaffected by the default change).
- `V-3` Live runner log confirms a Claude parent spawned a Codex child after the change; the user could no longer reproduce the missing-tool error.

## 9. Regression Guard

- tests: existing Claude/MCP suite covers the ready-gate behavior.
- alerts: the adapter logs `[claude-mcp] MCP connection did not become ready within timeout` when degradation occurs — a signal to investigate a slow/failing co-resident MCP server.

## 10. Follow-Up Document Updates

- upstream docs that must change: None.
- deferred: optionally isolate required FlowPilot MCP tools from optional co-resident servers so an indefinitely-hanging sidecar can never drop them.
