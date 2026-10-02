# CA-1103 — BUG-556: hub spawn_agent resolves the node's configured model

## What
`spawnChildRun` now resolves a flow node's step-configured model when a hub
ad-hoc `spawn_agent` call arrives with no model (the wire input cannot carry
one — `SpawnAgentInput.Model` is `json:"-"`). New helper
`spawnAgentNodeModel` matches the spawn against the parent's active flow
nodes by node ID (agent or label) or unique agent-ref basename, then applies
the same `resolveFlowNodeModel` the engine dispatch uses.

## Why
Live run-38799 (PrivateVault CP-02): the vibe-sprint reviewer step was
configured grok-4.7 but the hub's `spawn_agent` child ran devin/swe-2-high —
the step card and the child record showed different models for one leg.
`SpawnAgentInput.Model` is internal-only, so hub MCP calls always fell into
the parent-model inheritance fallback and the configured row was bypassed.

## Guarantees kept
- Explicit `in.Model` (engine dispatch, BUG-228) is still authoritative.
- Zero or ambiguous node matches inherit the parent model exactly as before.
- `resolveFlowNodeModel` behavior gates (spawnable behaviors only, CA-616
  planner guard, CA-358 flow-scoped rows) are reused unchanged.
- Provider parity: model resolution precedes provider selection;
  `providerKeyFromModel` pins the correct provider for the resolved model —
  the fix is provider-agnostic by construction.

## Tests
- `TestBug556_HubSpawnResolvesNodeConfiguredModel` — red before the fix
  (child spawned devin/swe-2-high), green after (grok-4.7).
- `TestBug556_HubSpawnWithoutMatchingNodeStillInherits` — non-node spawn
  keeps parent-model inheritance.
- `TestBug556_ExplicitSpawnModelStillWins` — explicit in.Model wins over
  the node row.
