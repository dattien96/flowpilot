# CA-1165 — run-2062497 D1: done verdict cannot seal a sprint with unvisited spine nodes

## What changed

`apps/local-runner/internal/runner/flow_executor.go`:

- `applyFlowControl` gains a `viaEngineEdge` flag. Both engine-internal
  done-callers (the inline edge-chain advancer and the settle path) mark
  their `done` submissions as engine-originated; operator/agent POSTs keep
  the field false.
- New `unvisitedSpineNodes`: walks the mandatory `done`-edge spine from the
  flow entry to the submitted node and returns spine members whose step row
  exists and was never dispatched (PENDING/WAITING). Missing rows are not
  flagged — `markFlowRunComplete` can only settle rows that exist, so an
  absent row cannot be fabricated.
- When an engine `done` would call `markFlowRunComplete` with unvisited
  spine nodes, the verdict is refused and escalated to a waiting state
  instead of sealing the sprint.

Live repro: sprint 3 stamped `coder/validate/spec_align/reviewer` SKIPPED
and `flow_run_complete_done` while the oracle was RED — a debate_synthesis
`done` on the terminal edge settled the whole graph with mandatory impl
nodes never executed.

## Invariant

A mandatory-chain node that was never dispatched cannot be settled by a
downstream `done` — the spine is the contract, and a seal without spine
evidence fails closed.

## Tests

`cp67_coder_transport_test.go` — `TestRun2062497_DoneVerdictRejectsUnvisitedSpine`,
`TestRun2062497_DoneVerdictSealsWhenSpineComplete`.
