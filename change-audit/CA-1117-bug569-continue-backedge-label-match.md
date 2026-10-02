# CA-1117 — BUG-569: continue back-edge matches task-scoped child labels

## Why

Live run-100368 (PrivateVault Task-025): a hub `continue` resolved to the
synthesis→coder back-edge, stamped the `coder` step RUNNING — and then
silently no-oped. `maybeReinvokeCoderForContinue` matched children by
exact label `coder`, but hub ad-hoc spawns carry task-scoped labels like
`task025_coder` (optionally `_rN` round suffixes). No child matched, no
leg was re-driven, and the step sat RUNNING forever — the flow only
moved because an operator spawned a `coder`-labeled leg by hand.

## What changed

`apps/local-runner/internal/runner/interactive_service.go`:

- New `childMatchesFlowNodeID(child, nodeID, node)`: exact label match,
  then a task-scoped variant match — strip a trailing `_r<digits>` round
  suffix and accept labels ending in `_<nodeID>` — then an agent-identity
  fallback (`agents/coder.md` matches a child whose agentName/role is
  `coder`) for hub spawns that never echoed the node id. The caller's
  newest-first scan keeps the preference ordered toward the latest leg
  (BUG-559's stale same-label leg protection is preserved).
- Both `reinvokeMatchingFlowChild` call sites in
  `maybeReinvokeCoderForContinue` (the agent.delegate path and the
  agent.code fallback) use the widened matcher when `targetNodeID` is
  known.
- On a total miss the path now emits a `continue_backedge_no_child`
  flow-diag event naming the target node instead of wedging silently.

## Invariant

A resolved back-edge target must re-drive a real child or make noise —
never stamp a step RUNNING and walk away.

## Tests

`bug569_continue_backedge_label_match_test.go` (red → green):

- synthesis `continue` back-edge re-drives a `task025_coder`-labeled
  child leg, not a silent no-op on a RUNNING step
