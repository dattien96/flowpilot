# BUG-556 — hub `spawn_agent` ignores the node's configured model; child inherits the run model

## Status
RESOLVED — spawnChildRun now fills an empty SpawnAgentInput.Model from the
matching active flow node's configured model (`spawnAgentNodeModel` →
`resolveFlowNodeModel`) when the parent run is flow-driven. Node match:
exact node-ID on agent or label, else unique agent-ref basename match;
ambiguous/zero matches inherit as before. Explicit in.Model (BUG-228) still
wins. Regression tests: `bug556_hub_spawn_node_model_test.go`.

## Original report
Live-found during PrivateVault CP-02 vibe run `run-38799`
(2026-10-02 ~07:03): the flow step card showed reviewer = `grok-4.7` but
`child_spawn_created` logged `provider=devin model=devin/swe-2-high` —
the hub's ad-hoc `spawn_agent` call produced a Devin reviewer while the
configured model sat in the step_definitions row.

## Root cause
`SpawnAgentInput.Model` is `json:"-"` — internal-only, never decoded from
the wire. The engine's own node dispatch fills it via
`delegateSpawnModel`/`resolveFlowNodeModel` (BUG-228), but a hub agent's
MCP `spawn_agent` call cannot — `parseSpawnAgentInput` has no model field
and nothing re-resolves the node's configured model from the parent's
active flow nodes. `spawnChildRun` then falls back to
parent-model → provider-default, so the child silently runs the hub's
model instead of the step-configured one.

## Fix
In `spawnChildRun`, when `in.Model` is empty and the parent run is
flow-driven, resolve the matching active flow node (exact node-ID/label
match, else unique `flowNodeAgentName` match — ambiguous matches stay on
inherit) and apply `resolveFlowNodeModel`, the same resolution the
engine dispatch path uses. Mirrors BUG-228 precedence; only fills the
currently-empty channel.

## Provider scope
Provider-agnostic: model name resolution happens before provider
selection; `providerKeyFromModel` then pins the right provider for the
resolved model (devin/grok/claude all exercised by the same path).
