# CA-1167 — run-2062497 D3+D4: sealed-loop spawn refusal and boundary auto-advance

## What changed

`apps/local-runner/internal/runner/interactive_service.go`:

- `spawnChildRun` refuses up-front when the parent loop is `done` or
  `stopped` (extending the BUG-432 `blocked` guard), before any child
  record exists. Live: operator remediation spawns between 06:59–07:03
  created three children on a sealed loop that each failed at turn
  admission with `flow_stopped`, leaving zombie `dispatch_failed` legs.
  CP-36: "loop ends, no further spawns."

`apps/local-runner/internal/runner/flow_executor.go` /
`agent_orchestrator.go`:

- `hasRunningSprintStep` is variadic — callers pass excluded node IDs.
- New `flowDoneEdgePredecessors` returns the direct `done`-edge predecessors
  of a node (vs the transitive `flowForwardDonePredecessors`).
- `maybeAutoAdvanceVibeSprintBoundary` accepts an exclusion set; the
  audit-settle and hub-settle call sites exclude the settle's direct
  predecessors. Live: auto-advance ran while `synthesis` was still RUNNING —
  but synthesis is the settle ancestor that `markFlowRunComplete` itself
  stamps DONE, so vetoing on it deadlocked the boundary for ~97s until an
  operator resume re-parked. The BUG-561 self-exclusion site keeps its
  semantics unchanged.

## Invariant

A sealed loop admits no ordinary spawns (boundary-mount re-drives remain
exempt via the unblock-first ordering), and the node being settled by the
very `markFlowRunComplete` in flight cannot veto the boundary advance it is
about to enable.

## Tests

`bugf_cluster_settle_resume_test.go` (`TestRun2062497_SpawnRefusedWhileParentLoopDone/Stopped`),
`vibe_sprint_boundary_test.go` (`TestRun2062497_AuditSettleAutoAdvanceIgnoresSettleAncestor`,
`TestRun2062497_RunningNonAncestorStillVetoesAutoAdvance`).
