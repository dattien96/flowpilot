# BUG-128: Inconsistent Agent Spawn Prompt Composition Across Providers

## Metadata

- Document ID: `BUG-128`
- Title: `Inconsistent Agent Spawn Prompt Composition Across Providers`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-125: Project Codex Agent Catalog Precedence](./BUG-125-Project-Codex-Agent-Catalog-Precedence.md), [CA-119: Provider-consistent agent spawn prompt composition](../../../change-audit/CA-119-provider-consistent-agent-spawn-prompt-composition.md)
- Replaces: `None`
- Tags: `agents, spawn, prompt, codex, claude, provider-parity, runner`

## AI Quick View

### Summary

- When spawning the same agent on Codex vs Claude, the prompt the child actually received looked different: Codex got the short built-in `coder` system prompt ("You are the coder sub-agent. Implement…") while Claude got the full project `.claude/agents/coder*.md` markdown body — both followed by the user's own child prompt.
- The composition code (`spawnChildRun`, `interactive_service.go`) was already identical for both providers (`agentDef.SystemPrompt + "\n\n" + in.Prompt`); the visible difference came entirely from the resolved `agentDef` being a different definition per provider (catalog precedence, see BUG-125), not from a provider-specific code branch.
- The agent definition was dumped inline with no consistent header and no link back to its source file, so the user could not tell which agent/definition produced the prompt and the shape varied with the definition's size.
- Fix: route all providers through one `composeAgentSpawnPrompt` helper that keeps the system prompt first (preserving built-in-agent prompt detection), then adds one consistent identity line naming the agent/role and linking its definition file path, then the user prompt.

### Current Ask

- Make the spawn prompt composition consistent across providers, explicitly mention the agent, and attach the agent definition file path so the AI can open the full spec itself.

### Key Decisions

- `V-1` One shared helper builds the spawn prompt for every provider; there is no provider-specific composition branch.
- `V-2` The agent system prompt stays first so `isAgentHistoryRun` / `hasBuiltInAgentPromptPrefix` (backend and frontend) keep classifying built-in agent runs by prompt prefix.
- `V-3` A single identity line names the agent and role and links the definition file (`definition: <path>`), or marks it `built-in (<source>)` when the agent has no on-disk path.

### Constraints

- Do not change how `agentDef` is resolved (catalog precedence is BUG-125's concern); only unify and label the composition.
- Do not break built-in-agent run detection, which relies on the prompt starting with `you are the <role> sub-agent.`.
- Keep the user's child prompt verbatim and last.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `spawnChildRun` composition site; new `composeAgentSpawnPrompt` / `composeAgentIdentityLine` helpers.
- `apps/local-runner/internal/runner/agent_catalog.go` — `AgentDefinition.Path` / `Source` used for the definition link.
- `apps/local-runner/internal/runner/interactive_handlers.go` — `hasBuiltInAgentPromptPrefix` (prefix detection that must keep matching).
- `apps/desktop-flowpilot/src/components/navigatorHistory.ts` — frontend twin of the prefix detection.

## 1. Issue Summary

A user spawned the same agent (`coder`) once on Codex and once on Claude, giving each a short child prompt ("You are codex agent. Just return: I like number 555" / "…number 999"). The prompt the child actually received differed by provider: on Codex it was prefixed with the short built-in `coder` system prompt, on Claude it was prefixed with the entire project `.claude/agents/coder*.md` markdown body. The inline dump had no consistent header and no pointer to the agent definition file, so the composition looked provider-specific and unauditable.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) (§7.3 item 2 updated)
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: FlowPilot local runner, a project with `.claude/agents/coder*.md` present and no equivalent `.codex/agents` definition, so Codex falls back to the built-in `coder`.
- reproduction steps:
  1. Spawn agent `coder` on Claude with prompt `You are claude agent. Just return: I like number 999`.
  2. Spawn agent `coder` on Codex with prompt `You are codex agent. Just return: I like number 555`.
  3. Inspect the first turn each child received: Claude's is prefixed with the full project markdown body; Codex's is prefixed with the one-line built-in system prompt.
- frequency: always when the resolved definition differs between providers (the common case once a project ships only `.claude/agents`).

## 4. Expected vs Actual

- expected: the same agent name produces the same prompt shape on every provider, the agent is named explicitly, and the agent definition file is linked so the model can read the full spec.
- actual: the prompt shape varied with the resolved definition, the agent was not named in a consistent header, and the definition file was not linked.

## 5. Impact

- users affected: anyone spawning agents across providers (the multi-agent feature's core path); future auto/talk-agent mode relies on a predictable, provider-independent prompt shape.
- workflows affected: every `spawn_agent` tool call and UI spawn that resolves a non-nil agent definition.
- severity: Medium — no crash, but inconsistent and unauditable agent context across providers undermines multi-agent parity.

## 6. Root Cause

- hypothesis: a provider-specific composition branch.
- confirmed cause: there is no provider-specific branch; `spawnChildRun` always did `agentDef.SystemPrompt + "\n\n" + in.Prompt`. The visible difference came from `agentDef` resolving to different definitions per provider (catalog precedence, BUG-125), combined with an inline dump that had no consistent header and no link to the source file, so the same agent looked different per provider and could not be traced to its definition.
- evidence: code inspection of `interactive_service.go:1226-1229` (single composition line for all providers) and `agent_catalog.go` precedence (`.claude/agents` + `.codex/agents` > provider-home > built-ins); the built-in `coder` system prompt is the short string, the project file body is the full markdown.

## 7. Fix Strategy

- `F-1` Add `composeAgentSpawnPrompt(agentDef, userPrompt)` and `composeAgentIdentityLine(def)` in `interactive_service.go`; route the `spawnChildRun` composition through `composeAgentSpawnPrompt` for every provider.
- `F-2` Keep the agent system prompt first; append one identity line `[FlowPilot sub-agent — agent: <name> | role: <role> | definition: <path|built-in (<source>)>]`; then the user prompt verbatim.
- `F-3` Return the user prompt unchanged when the agent definition is nil (unknown agent).
- `F-4` Record the provider-independent composition rule in SD-16 §7.3 item 2.

## 8. Validation

- `V-1` `go test ./internal/runner/ -run TestComposeAgentSpawnPromptIsProviderConsistent -count=1` — pass (system prompt first, identity line with linked definition, user prompt last, built-in marker, nil pass-through).
- `V-2` `go test ./internal/runner/ -run 'TestUISpawnInjectsContextIntoParentProviderTurn|TestToolSpawnWaitTrueDoesNotInjectParentContext|TestToolSpawnWaitFalseInjectsResult' -count=1` — pass (spawn/injection paths unaffected).
- `V-3` No-regression check: `go test ./internal/runner/ -run 'Agent|Spawn|Catalog|History|Resume|Codex' -count=1` with and without the change — failure set identical (same 10 pre-existing environment failures: real-`codex`-binary resume, compat/cross-account, provider-home skill merges); the change adds exactly one passing test and zero new failures.
- `V-4` `go build ./internal/runner/...` — pass.

## 9. Regression Guard

- tests: `TestComposeAgentSpawnPromptIsProviderConsistent` asserts the shared shape and that the system prompt stays first.
- alerts: built-in agent runs vanishing from agent-history detection would indicate the system prompt is no longer first in the composition.
- audit checks: any new provider-specific spawn-prompt branch must be rejected; all providers must flow through `composeAgentSpawnPrompt`.

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-16 §7.3 item 2 — updated to state the provider-independent composition rule.
- notes left unchanged on purpose: catalog precedence behavior (BUG-125) is unchanged; this delta only unifies and labels the composition, it does not change which definition is resolved.
