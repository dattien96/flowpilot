# BUG-125: Project Codex Agent Catalog Precedence

## Metadata

- Document ID: `BUG-125`
- Title: `Project Codex Agent Catalog Precedence`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `none`
- Related Documents: [Task-081: Agent Catalog And Picker](../../08-Task/done/Task-081-Agent-Catalog-And-Picker.md), [BUG-124: Codex Spawn-Agent Reserved Tool Name](BUG-124-Codex-Spawn-Agent-Reserved-Tool-Name.md)
- Replaces: `none`
- Tags: `agents, codex, catalog, precedence, project, account`

## AI Quick View

### Summary

- The agent picker ignored project-local Codex agent definitions stored as `.codex/agents/*.toml`.
- The catalog only parsed Markdown, although Codex's native agent format is TOML.
- Account discovery also merged every account home, allowing root `~/.codex` definitions to win over the active selected account.
- Fix: parse Codex TOML and enforce project > active provider account > built-in precedence.

### Current Ask

- Completed: show project-local Codex agents and prefer them over same-name active-account agents.

### Key Decisions

- `V-1` Continue supporting Claude/YAML-frontmatter Markdown agents.
- `V-2` Add native Codex TOML fields including model, reasoning effort, tools, and developer instructions.
- `V-3` Read provider-home agents only from each provider's active connected account once the runner is attached.
- `V-4` Keep case-insensitive first-seen name deduplication so project definitions remain authoritative.

### Constraints

- Do not change child-agent spawning semantics beyond which definition is selected.
- Preserve built-in fallback agents.
- Ignore malformed TOML files rather than breaking the complete catalog.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/agent_catalog.go`
- `apps/local-runner/internal/runner/interactive_service.go`
- Project files `.codex/agents/coder-agent.toml` and `.codex/agents/reviewer-agent.toml`

## 1. Issue Summary

The agent list displayed an account or built-in agent even when the selected project contained a same-name Codex agent definition.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: Desktop FlowPilot with project and account Codex agents under `.codex/agents`.
- reproduction steps:
  1. Create the same agent name in the selected project's `.codex/agents/*.toml` and an account `agents/*.toml`.
  2. Give the definitions different model values.
  3. Open the agent picker.
- frequency: always for native Codex TOML definitions before this fix.

## 4. Expected vs Actual

- expected: the project definition is shown and used; without a project copy, the active account definition is used.
- actual: project TOML was ignored, and account discovery could prefer an inactive root account.

## 5. Impact

- users affected: users with project-scoped Codex sub-agent definitions.
- workflows affected: agent picker display and child-agent model/instruction selection.
- severity: High because the wrong model can prevent child-agent startup.

## 6. Root Cause

- hypothesis: duplicate-name ordering was incorrect.
- confirmed cause: ordering was project-first, but `agentsFromDir` accepted only `.md`; native `.toml` definitions never entered the merge. Provider discovery also scanned all account homes instead of the selected active account.
- evidence: the live endpoint returned built-ins while project `.codex/agents/*.toml` files existed; focused tests pass after TOML parsing and active-account filtering.

## 7. Fix Strategy

- `F-1` Parse both `.md` and `.toml` agent files.
- `F-2` Map Codex TOML `developer_instructions` to the agent system prompt and preserve model settings.
- `F-3` Bind provider-home catalog discovery to active connected provider accounts during runner attachment.
- `F-4` Add a regression test where project and account TOML files share a name and the project model must win.

## 8. Validation

- `V-1` Focused agent-catalog and child-spawn tests pass.
- `V-2` Full runner test suite passes.
- `V-3` Live `/client/agents?cwd=<project>` returns project TOML paths and model settings.
- `V-4` `git diff --check` passes.

## 9. Regression Guard

- tests: `TestAgentCatalogProjectCodexTomlOverridesAccountToml` plus existing Markdown precedence tests.
- alerts: a project Codex agent showing `source=provider` or a provider-home path indicates precedence regression.
- audit checks: duplicate names must resolve in project, active-account, built-in order.

## 10. Follow-Up Document Updates

- upstream docs that must change: none; documented precedence already required project-local definitions to win.
- notes left unchanged on purpose: agent model compatibility remains provider-owned and is not validated by the catalog.
