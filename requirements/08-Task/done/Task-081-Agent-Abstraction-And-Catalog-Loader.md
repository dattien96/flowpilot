# Task-081: Agent Abstraction And Catalog Loader

## Metadata

- Document ID: `Task-081`
- Title: `Agent Abstraction And Catalog Loader`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](./Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md)
- Replaces: `None`
- Tags: `multi-agent, local-runner, provider-adapter, agent-catalog, go`

## AI Quick View

### Summary

- Extend `interactiveRun` with additive agent-identity fields so a run can represent a sub-agent without changing single-agent behavior.
- Add an `AgentDefinition` type and an `AgentCatalog` that discovers agent definitions from `.claude/agents/`, `.codex/agents/`, and FlowPilot built-ins, mirroring skill discovery in `interactive_catalog.go`.
- Expose a `listAgents` call in the runner contract so the desktop spawn dialog can populate the agent list.

### Current Ask

- Implement the foundational agent identity + on-disk agent catalog with `project > provider-home` precedence and a non-empty built-in set, plus unit tests. No spawning or UI yet.

### Key Decisions

- `T-1` Identity fields on `interactiveRun` (`parentRunId`, `agentName`, `role`, `dependsOn[]`, `agentStatus`) are additive and default to a parentless "main" run.
- `T-2` Agent definition format is the standard frontmatter (`name`, `description`, `tools`, `model`, provider hint) + system-prompt body; source is `claude | codex | flowpilot`.
- `T-3` Catalog precedence is project-local over provider-home; built-ins (coder, reviewer, tester) ship in-repo.

### Constraints

- Run GitNexus impact analysis before editing `interactiveRun` or any catalog/discovery function; warn on HIGH/CRITICAL.
- Do not alter the `ProviderRuntimeAdapter` interface or SSE contract in this task.
- `.codex/agents/` and `.claude/agents/` are empty today; the loader must tolerate missing/empty dirs.

### Open Questions

- Should built-in agent definitions live under `.agents/agents/` or a dedicated runner-embedded path?

### Source Refs

- `CP-19` P-1, P-3; `interactive_catalog.go`; `provider_registry.go` (`interactiveRun`).

## 1. Goal

Establish the agent abstraction (identity on `interactiveRun`) and an `AgentCatalog` disk loader so later tasks can spawn and route agents, with the catalog populated by built-ins and any on-disk definitions.

## 2. Parent Links

- coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `CP-19` P-1, P-3

## 3. Trigger

CP-19 requires a sub-agent to be a tagged `interactiveRun` and requires agent definitions to load from disk; both primitives are missing today.

## 4. Exact Change

- `T-1` Add `parentRunId string`, `agentName string`, `role string`, `dependsOn []string`, `agentStatus string` to `interactiveRun` with safe defaults.
- `T-2` Add `AgentDefinition` struct + `AgentCatalog` with discovery from `.claude/agents/`, `.codex/agents/`, provider-home, and built-ins; implement precedence and parsing of frontmatter + body.
- `T-3` Ship built-in `coder`, `reviewer`, `tester` definitions in-repo.
- `T-4` Add `listAgents(cwd?)` to the runner HTTP surface and the `RunnerClient` contract type.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/interactive_service.go`, new `agent_catalog.go`, `interactive_handlers.go`, `apps/desktop-flowpilot/src/types/contract.ts`
- modules: local-runner interactive
- routes: `GET /client/agents`
- tables: none

## 6. Acceptance Check

- `AgentCatalog` returns built-ins when dirs are empty and applies `project > provider-home` precedence when both define the same agent name (unit tested).
- A parentless run still behaves exactly as today (no regression in normal chat).
- `listAgents` returns the expected definitions over HTTP.

## 7. Out of Scope

- Spawning, the orchestrator, any UI, and Supabase changes (Tasks 082–085).

## 8. Completion Notes

- result: Implemented. `AgentDefinition` + `AgentCatalog` added in `apps/local-runner/internal/runner/agent_catalog.go` (discovers `.claude/agents` + `.codex/agents`, provider homes, with `coder`/`reviewer`/`tester` built-ins as the non-empty fallback; `project > provider-home > built-in` precedence, frontmatter + body parsing). `interactiveRun` extended with additive identity fields (`parentRunID`, `agentName`, `role`, `dependsOn`, `agentStatus`). Service wired with `agentCatalog` + `GET /client/agents` handler/route. Contract gained `AgentDefinition` + optional `RunnerClient.listAgents`, implemented in `HttpWsRunnerClient`. Tests: `agent_catalog_test.go` (built-ins, project-local override precedence, codex discovery + comma tools, HTTP endpoint) — all pass.
- verification: GitNexus impact on `interactiveRun` = LOW (3 direct callers, 0 processes). `go build ./...` + `go vet` clean; desktop `tsc --noEmit` exit 0. Full runner suite: the change adds zero new failures vs. the clean tree (14 pre-existing env-dependent failures: missing `codex` binary, Windows `\tmp` path parsing, absent provider-home fixtures; `TestProjectRunHistoryFiltersRunsByProject` is pre-existing-flaky and passes in isolation).
- follow-ups: Task-082 consumes `AgentCatalog` + identity fields for `spawn_agent`. Consider shipping example `.claude/agents/*.md` files for discoverability (optional). Make `listAgents` a required `RunnerClient` method in Task-083 once the mock client implements it.
- upstream docs updated: CP-19 (P-8 + Source Refs) and this task reflect the mode-agnostic Agents panel decision.
