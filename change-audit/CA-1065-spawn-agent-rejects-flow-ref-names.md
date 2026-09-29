# CA-1065 — spawn_agent refuses flow-definition names (R.3#3 option c)

## Symptom (design review → approved option)

The `spawn_agent` tool accepts any `agent` string and resolves it against the
agent catalog only. Passing a flow id — `"vibe-sprint"`,
`"flowpilot-core-flow-pack/vibe-sprint"`, `"review-loop"`, etc. — misses the
catalog, leaves `agentDef` nil, and still mints a child run stamped
`agentName:"vibe-sprint"`. That child runs the raw prompt as a plain chat
turn: **no flow mount, no contract freeze, no TDD gate chain, no sprint
nodes** — while the UI presents it as a sprint child. Two different
"sprint" execution modes thus coexisted and were presented as equivalent
(R.2 design observation #3; verdict: pending product decision → operator
approved option **(c) fail-closed**).

## Fix

`spawnChildRun` (interactive_service.go): when the agent catalog misses
(`agentDef == nil` and no `AgentDefOverride`), the input is resolved as a
flow ref via `NewFlowDefinitionResolver(s.flowDefinitionStore).
ResolveFlowRef`. A positive resolution — valid record OR
`ErrFlowDefinitionInvalid` (a flow that exists but fails validation is still
a flow, not an agent) — returns a typed refusal:

> `"<name>" is a flow definition, not an agent — spawn_agent only accepts
> agent names; gated flows are mounted by the flow engine`

Resolution *errors* (store down, genuinely unknown name) are not refusals:
unknown non-flow names keep the existing degrade-to-raw-prompt child
contract untouched. Internal executor spawns carry `AgentDefOverride` and
never reach the check.

## Tests

`internal/runner/spawn_agent_flow_ref_guard_test.go` (3 tests, RED→GREEN):

- bare builtin ids `vibe-sprint` / `review-loop` / `vibe-owner-debate` are
  refused with a message naming the flow contract;
- the pack-qualified form `flowpilot-core-flow-pack/vibe-sprint` is refused;
- an unknown non-flow name still mints a raw-prompt child (existing
  contract preserved — the guard only closes the flow-ref hole).

## Provider parity

Provider-agnostic: the guard runs in `spawnChildRun` before provider
selection — every provider's spawn_agent MCP/HTTP path shares it.
