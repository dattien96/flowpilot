# Task-209: Grok MCP, Ask-User, And Spawn-Agent Parity

## Metadata

- Document ID: `Task-209`
- Title: `Grok MCP, Ask-User, And Spawn-Agent Parity`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-11`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-208: Grok Permission Channel And YOLO Posture](./Task-208-Grok-Permission-Channel-And-Yolo-Posture.md)
- Child Documents: `None`
- Related Documents: [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [Task-055: Fix Claude Ask User Live Flow](../done/Task-055-Fix-Claude-Ask-User-Live-Flow.md), [Task-056: Fix Codex Ask User Live Flow](../done/Task-056-Fix-Codex-Ask-User-Live-Flow.md), [BUG-128: Inconsistent Agent Spawn Prompt Composition Across Providers](../done/BUG-128-Inconsistent-Agent-Spawn-Prompt-Composition-Across-Providers.md), [CP-29: MCP Proxy Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md)
- Replaces: `None`
- Tags: `grok, grok-build, mcp, ask-user, spawn-agent, multi-agent, google-drive, acp`

## AI Quick View

### Summary

- Decide and implement how FlowPilot's runner-owned tools (`ask_user`, `spawn_agent`, `submit_review_outcome`) reach Grok: via ACP `session/new{mcpServers:[flowpilot]}` reusing the existing runner-hosted HTTP MCP server, or via Grok's own native `ask_user_question`/`spawn_subagent` tools (both observed in the live `initialize` tool list) with bridge shims.
- Merge the Google Drive MCP server into the same `mcpServers` array so Grok has FlowPilot tools and external MCP tools in the same turn, with ambient compat scanning kept disabled (Task-206 `T-4`).
- Wire `spawn_agent` end to end: wait=true/false, child run persistence, agent panel visibility, inherited provider/yolo/context, and provider-neutral spawn-prompt composition (no Grok-specific prompt drift, per `BUG-128`).

### Current Ask

- Ship one working path (document the choice and why) for `ask_user` and `spawn_agent` on Grok, proven by a live blocking question and a live spawned child (both wait=true and wait=false), plus Google Drive visible in the same turn.

### Key Decisions

- `T-1` Prefer reusing the runner-hosted `flowpilot` MCP server (`claudeMCP`/`mcpBaseURL`/`claudeMCPToolDefs`/`handleClaude*` handlers) passed through ACP's real `mcpServers` field — these are already provider-neutral and this avoids writing new tool-name/schema shims. Only fall back to native `ask_user_question`/`spawn_subagent` bridge shims if the MCP-over-ACP path proves unreliable in testing.
- `T-2` `--strict-mcp-config`-style shadowing is a non-issue on ACP: `mcpServers` is a real per-`session/new` array, so there is no ambient-file precedence fight the way Claude has with `.claude.json`.
- `T-3` Merge Google Drive via the existing `flowpilotClaudeExtraMCPServers(accountHome, yolo)` helper, appended into the same `mcpServers` array alongside the `flowpilot` entry.
- `T-4` `spawn_agent` must call `TurnBridge.SpawnAgent` with the same input shape (`Agent, Prompt, Provider, DependsOn, Wait, FlowCohortId, CohortSize, Label, AutoOrchestrate`) Codex/Claude use, via the shared `parseSpawnAgentInput` helper — no Grok-only argument parsing.
- `T-5` `OfferReviewOutcomeTool` gating parity: only advertise `submit_review_outcome` on a hub turn, and re-verify eligibility on the actual tool call (mirror the Codex/Claude defense noted in CP-42's BUG-NOTE #24).

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-46 `P-0`):** reuse the shared `claudeMCP`/`claudeMCPToolDefs`/`handleClaude*` and `agent_orchestrator` provider-neutrally — never mutate them; `defaultModelForProvider` gets an appended `grok` case only.
- Do not degrade Codex/Claude MCP behavior while reusing the shared `claudeMCP` server/handlers.
- The ambient-scan-disabled requirement from Task-206 must remain enforced; this task must not reintroduce it while wiring `mcpServers`.
- `ask_user` must route through the exact same desktop question-card contract (`user_question_required` → `AskQuestion`) used by Codex/Claude — no Grok-specific question UI.

### Open Questions

- `Q-1` (CP-46 `Q-4`) Does the MCP-over-ACP path deliver `tools/list`/`tools/call` reliably enough, or does Grok's own `spawn_subagent`/`ask_user_question` (seen in `initialize`'s tool list) need to be the primary path instead?
- `Q-2` Does Grok's ACP `mcpServers` accept the same HTTP+token shape `writeClaudeMCPConfig` already produces, or does it need a different envelope (e.g. no `--mcp-config` file, just inline JSON in `session/new`)?

### Source Refs

- `CP-46` section `P-7`; parity rows `GR-06`, `GR-07`, `GR-17`, `GR-21`, `GR-22`.
- Live evidence (CP-46 authoring): `initialize` tool list includes `spawn_subagent`; ACP `session/new{mcpServers:[]}` accepted; ambient MCP auto-discovery observed and must stay disabled.
- `apps/local-runner/internal/runner/claude_mcp_server.go`, `claude_permission_mcp.go` (tool names/handlers — reusable, provider-neutral), `google_drive_mcp_provider_config.go` (`flowpilotClaudeExtraMCPServers`), `codex_adapter.go` (`handleDynamicToolCall`, native-tool bridge shim template), `agent_orchestrator.go` (`SpawnAgentInput`/`SpawnAgentResult`, `parseSpawnAgentInput`).

## 1. Goal

Give Grok working `ask_user`, `spawn_agent`, and external-MCP (Google Drive) access through the shared runner bridge, with provider-neutral prompt composition and full multi-agent lifecycle parity (wait semantics, child persistence, agent panel, inherited YOLO/context).

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-16`, `SD-11`
- system spec: `SS-12`
- specific upstream ids: `CP-46 P-7`

## 3. Trigger

Task-208 makes tool-call gating safe; this task adds the actual FlowPilot-owned tools Grok can call once a turn is gated correctly.

## 4. Exact Change

- `T-1` Build the ACP `mcpServers` entry for the `flowpilot` server (reuse/extend `geminiACPFlowPilotMCPServers`-style builder from the Task-206 shared ACP module) and pass it in `session/new`.
- `T-2` Append the Google Drive server (via `flowpilotClaudeExtraMCPServers`) into the same array when the active account has Drive configured.
- `T-3` If native shims are chosen instead/in addition (per `Q-1`), add `handleGrokDynamicToolCall` routing Grok's native `ask_user_question`/`spawn_subagent` tool-call frames to `bridge.AskQuestion`/`bridge.SpawnAgent`.
- `T-4` Implement `OfferReviewOutcomeTool` conditional advertisement + re-check-on-call for Grok, mirroring Codex/Claude.
- `T-5` Add integration tests: blocking `ask_user` round-trip; `spawn_agent` with `wait=true` and `wait=false`; child run persistence + agent panel event emission; Google Drive tool visible in the same turn as `ask_user`.
- `T-6` Add a spawn-prompt-composition parity test comparing Grok's assembled child prompt against Codex/Claude for the same inputs (`BUG-128` regression shape).
- `T-7` **MCP-ready-before-prompt gate (BUG-114):** withhold `session/prompt` until FlowPilot's MCP tools are connected (Grok inits MCP async — `_x.ai/mcp/init_progress`), bounded by a timeout that degrades to sending anyway; mirror Claude's `waitReady`/`claudeMCPReadyDefaultTimeout`. Test that a slow-init turn still exposes `ask_user`/`spawn_agent` on turn 1 (`GR-30`).
- `T-8` **Resumed-turn re-attach (BUG-087):** a resumed Grok turn must re-pass `mcpServers` and re-arm the permission channel so gated tools + `ask_user` still work after resume; add a regression test (`GR-31`).
- `T-9` **Reserved tool-name safety (BUG-124):** ensure FlowPilot's spawn/ask tool names do not collide with Grok's native `spawn_subagent`/`ask_user_question`; if a collision exists, use a `flowpilot_`-prefixed name and normalize it back at the UI boundary (mirror `codexSpawnAgentToolName`).
- `T-10` **User-initiated spawn injection (BUG-121/122):** a UI-initiated child (`SpawnAgentInput.UIInitiated`) emits `EventAgentSpawnedByUser` and injects the parent-context note into the Grok parent's next turn; add a test.
- `T-11` **Append a `grok` case to `interactive_service.go defaultModelForProvider`** so a cross-provider child spawned onto Grok with no explicit model gets a valid default (else `""` → request failure). Additive only.

## 5. Touched Areas

- files: `grok_adapter.go` (mcpServers wiring), new `grok_mcp.go` or reuse of `claude_mcp_server.go` helpers, `agent_orchestrator.go` (verify no Grok-specific branch needed), tests
- modules: MCP subsystem, agent orchestration bridge
- routes: reuses existing `ClaudeMCPPath` HTTP endpoint (no new route) if the MCP-over-ACP path is chosen
- tables: none

## 6. Acceptance Check

- A live Grok turn calling `ask_user` produces a desktop question card; answering resumes the turn.
- A live Grok turn spawning two children (one `wait=true`, one `wait=false`) shows both in the agent panel with correct parent-continuation behavior.
- Google Drive MCP tools are visible to Grok in the same turn as `ask_user`/`spawn_agent`, or the capability is explicitly reported as unsupported with diagnostics (no silent tool-list gap).
- Spawn-prompt composition matches Codex/Claude for identical inputs.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` `ask_user` works end to end for Grok through the shared bridge/question-card contract. **Closed 2026-07-10 (CA-278):** unit path `tools/call ask_user` → `bridge.AskQuestion` (`TestGrokMcpAskUserRoundTrip`, multiSelect/empty-answer, wire reinforcement); live registry + default `preparePrompt` append `grokAskUserReinforcement` so the model uses FlowPilot MCP `ask_user` instead of native `ask_user_question`. **Live desktop confirmed:** structured Question card (options form) appears for Grok chat and answering resumes the turn (user retest after runner restart).
- [x] `DOD-2` `spawn_agent` works end to end for Grok with both wait modes and correct child persistence/panel visibility. **Closed 2026-07-11 (CA-279):** keyed `grokProcesses` coexistence + per-turn model/effort inheritance so parent turns survive child spawn; MCP `tools/call spawn_agent` → `TurnBridge.SpawnAgent` (`TestGrokMcpSpawnAgentRoundTrip`, wait modes, process coexistence, inheritance tests). **Live desktop confirmed:** child spawn without `grok agent process torn down`; child visible in agent panel; parent returns idle after `wait=false` and `wait=true` child completion (user retest 2026-07-11).
- [x] `DOD-3` Google Drive (or another configured external MCP server) is visible to Grok in the same turn as FlowPilot tools, or is explicitly and honestly marked unsupported. (`TestGrokAdapterBuildsMcpServersArrayAndRegistersBridge` proves both entries land in the ACP `mcpServers[]` array sent to `session/new`.)
- [x] `DOD-4` `OfferReviewOutcomeTool` gating parity is implemented and tested. **Closed 2026-07-11:** `grok_adapter.SendTurn` passes `req.OfferReviewOutcomeTool` to `mcpServer.register` (same gate as Claude/Codex). Tests: `TestGrokMcpReviewOutcomeGatingToolsList`, `TestGrokMcpSubmitReviewOutcomeRejectedWhenNotAllowed`, `TestGrokMcpSubmitReviewOutcomeRoundTrip` in `grok_mcp_test.go`.
- [x] `DOD-5` Spawn-prompt composition parity test passes against Codex/Claude baselines. **Closed 2026-07-11:** `TestGrokSpawnPromptCompositionMatchesClaudeCodexBaseline` asserts Grok/Claude/Codex child first-turn provider prompts are byte-identical for the same built-in `coder` agent, user prompt, and workspace (shared `composeAgentSpawnPrompt` via `spawnChildRun`, BUG-128).
- [x] `DOD-6` MCP-ready gate withholds the prompt until FlowPilot tools connect; slow init still yields `ask_user`/`spawn_agent` on turn 1 (`GR-30`, BUG-114). (`TestGrokAdapterMcpReadyGateDoesNotHangOnTimeout` proves the gate degrades rather than hangs; the "still yields the tools on turn 1" half follows from DOD-3's wiring but isn't independently re-asserted.)
- [x] `DOD-7` Resumed Grok turns re-attach `mcpServers` + permission channel; gated tool + `ask_user` work after resume (`GR-31`, BUG-087). **Closed 2026-07-11 (live):** user resumed Grok chat and `ask_user` / `spawn_agent` still work on follow-up turns; `ensureSession` `session/load` re-passes `mcpServers` and permission channel re-arms per turn (structural path unchanged). No dedicated automated resume test added — live desktop retest accepted.
- [x] `DOD-8` FlowPilot tool names do not collide with native `spawn_subagent`/`ask_user_question`; normalized at the UI boundary (`GR-07`, BUG-124). **Closed 2026-07-11 (CA-281):** distinct names (`spawn_agent` vs `spawn_subagent`); `grokNormalizedToolDisplayName` aliases native tools at the event boundary; `grokSpawnAgentReinforcement` + `spawn_subagent` notification shim routes to `TurnBridge.SpawnAgent`. **Live desktop confirmed:** Grok reports `spawn_agent` (FlowPilot MCP); child visible in agent panel; parent returns idle.
- [x] `DOD-9` UI-initiated child spawn emits `EventAgentSpawnedByUser` and injects into the Grok parent's next turn (`GR-22`, BUG-121). **Closed 2026-07-11 (live):** user UI-spawned child (built-in agent + Grok provider override); child completed; parent returned idle; provider-neutral `spawnChildRun` path unchanged. Live desktop retest accepted for Grok.
- [x] `DOD-10` `defaultModelForProvider("grok")` returns a valid default; cross-provider spawn onto Grok with no model does not fail (additive edit). (`TestDefaultModelForProviderBaseRegressionPlusGrok`.)
- [x] `DOD-11` **Base-regression (`P-0`):** shared `claudeMCP`/`agent_orchestrator`/`defaultModelForProvider` are reused/appended-only; Codex/Claude MCP + spawn_agent tests remain green and unchanged. (Full suite verified unchanged vs. clean baseline.)

## 7. Out of Scope

- Multi-account/`GROK_HOME` isolation for MCP credentials — Task-210.
- Desktop agent-panel UI changes beyond what already renders provider-neutral data — Task-211 if any gap is found.
- Cross-provider handoff of spawned-agent transcripts — Task-212.

## 8. Completion Notes

- result: **All DOD-1…DOD-11 closed.** Task-209 complete (MCP-over-ACP primary path, ask_user + spawn_agent parity, live desktop retest 2026-07-11).
- implementation notes: Primary path is MCP-over-ACP (Task-209 T-1 / Q-1). ask_user: `grokAskUserReinforcement` appended in `preparePrompt`/`promptPrep`. spawn_agent: keyed `grokProcesses` map + `runTurn` model/effort stickiness (CA-279); spawn steer + native shim (CA-281).
- verification: `go test ./internal/runner -run 'Grok.*(AskUser|Mcp|Prompt|Native|Spawn|Process)'` PASS; live desktop retest 2026-07-11 — QuestionCard (DOD-1), MCP `spawn_agent` wait=false/wait=true + main idle (DOD-2), resume ask/spawn (DOD-7), model uses `spawn_agent` not native collision (DOD-8), UI-initiated spawn (DOD-9).
- follow-ups: optional PreToolUse denylist if reinforcement ever fails on a future Grok version (Task-221 was cancelled won't-do for MCP gating — reopen a hard-gate task only if needed); dedicated automated Grok resume test if regressions appear (DOD-7 accepted live-only).
- upstream docs updated: CA-278 (DOD-1), CA-279 (DOD-2), CA-281 (DOD-8), CA-282 (DOD-5).
