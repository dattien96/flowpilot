# Task-174: AgentPack Install And Agent Catalog Loading

## Metadata

- Document ID: `Task-174`
- Title: `AgentPack Install And Agent Catalog Loading`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-01`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `Task-177`, `Task-178`
- Related Documents: `Task-173`, `CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration`, `CP-41-RAG-Harness-Flow-Mode`
- Replaces: `N/A`
- Tags: `agent-flow-engine, agentpack, agent-catalog, local-runner`

## AI Quick View

### Summary

- Wire the parsed internal agent pack into the runner's agent catalog without changing flow execution yet.
- Built-in agent markdown files become the source for default coder, reviewer, and synthesizer agents.
- Preserve current behavior with fallback to existing Go built-ins until parity is proven.

### Current Ask

- Make internal pack agents discoverable by catalog lookup, while keeping user/project overrides first.

### Key Decisions

- `T-1` Built-in markdown can define persona and output guidance, but Go must still enforce state transitions.
- `T-2` Catalog precedence must be deterministic: project/user definitions override internal pack definitions.
- `T-3` Existing hardcoded built-ins stay as emergency fallback in this task.

### Constraints

- Do not enable pack flow execution yet.
- Do not remove existing built-in agent code until Task-180.
- Do not make internal pack files user-editable in place.

### Open Questions

- Exact user override locations should follow current `agent_catalog.go` conventions.

### Source Refs

- `apps/local-runner/internal/runner/agent_catalog.go`
- `apps/local-runner/internal/agentpack/flow-pack/agents/coder.md`
- `apps/local-runner/internal/agentpack/flow-pack/agents/reviewer.md`
- `apps/local-runner/internal/agentpack/flow-pack/agents/synthesizer.md`

## 1. Goal

Load built-in agent definitions from the internal agent pack so future behavior changes can happen by editing pack data rather than modifying Go constants.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `Task-173`, `CP-36`, `CP-42`

## 3. Trigger

The current built-in agent behavior is too coupled to Go code. Chat Mode can support built-in Review Loop, but the role prompts should live in pack data and be replaceable without a runner code change.

## 4. Exact Change

- `T-1` Embed internal pack files with `go:embed` or equivalent read-only runtime loading.
- `T-2` Add `agentpack.Provider` or equivalent adapter that exposes built-in agent definitions to the existing catalog.
- `T-3` Extend agent catalog lookup order:
  - project/workspace agent definitions
  - user/global agent definitions
  - internal agent pack definitions
  - legacy Go fallback
- `T-4` Map pack agent IDs to current agent catalog keys.
- `T-5` Include pack metadata in returned catalog entries:
  - `source=builtin-pack`
  - `packId`
  - `packVersion`
  - `editable=false`
- `T-6` Add tests for lookup precedence and fallback behavior.
- `T-7` Add a migration note in code comments showing that legacy Go built-ins are temporary until Task-180.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/agent_catalog.go`
  - `apps/local-runner/internal/agentpack/**/*.go`
  - `apps/local-runner/internal/agentpack/flow-pack/agents/*.md`
- modules:
  - local runner agent catalog
  - internal agentpack
- routes:
  - none
- tables:
  - none

## 6. Acceptance Check

- Built-in `coder`, `reviewer`, and `synthesizer` can be loaded from pack markdown.
- Workspace/user agent with same ID overrides internal pack agent.
- If pack loading fails, legacy fallback preserves current behavior and logs a clear warning.
- Unit tests cover precedence and missing pack entry fallback.
- No Chat Mode or Flow Mode routing changes yet.

## 7. Out of Scope

- Flow mirror sync.
- UI picker.
- Context package handoff refactor.
- Removing legacy built-in constants.

## 8. Completion Notes

- result: partially implemented — corrected 2026-07-06 (previously read "implemented" without qualification). Confirmed: coder/reviewer/synthesizer (+tester) load from pack markdown, project/provider-home overrides still take precedence, tests pass (`TestAgentCatalogReturnsBuiltinsWhenEmpty`, `TestSynthesizerBuiltinIsDiscoverable`, `TestAgentCatalogProjectLocalOverridesBuiltin`, `TestSynthesizerIsOverridableByProjectLocalFile`, `TestAgentCatalogProjectLocalOverridesProviderHome`, `TestListAgentsOverHTTP`), `go build ./...` clean. Two confirmed gaps: (1) `AgentDefinition` never gets pack-provenance metadata (`source=builtin-pack`/`packId`/`packVersion`/`editable=false` per T-5) — pack-sourced and legacy-Go-sourced built-ins are indistinguishable (`Source` is `"flowpilot"` either way); (2) when pack loading fails, the fallback to the old hardcoded Go agent list is silent — no warning is logged, and no test exercises that failure path.
- follow-ups: `Task-177`, `Task-180`, plus the two gaps above (pack-provenance metadata on `AgentDefinition`; logged warning + test for the pack-load-failure fallback).
- upstream docs updated: [CP-42](../../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes and [CA-147](../../../change-audit/CA-147-agent-flow-pack-and-generic-node-behavior-refactor.md)
