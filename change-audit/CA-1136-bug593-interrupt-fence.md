# CA-1136 — BUG-593: Interrupt was ctx-cancel only, no durable stop fence

## What changed

`apps/local-runner/internal/runner/interactive_service.go` (`Interrupt`):

- Wrote the durable run-stop fence (`requestRunStopV2`) for the run AND
  every registered child BEFORE the in-flight ctx cancels — the same
  linearization point `stopAgentLoop` already uses.
- Fence failure fails closed (`500 stop_fence_failed`) instead of returning
  "cancelling" while the dispatch CAS stays open.
- Follow-up turns release the hub fence via the existing
  `releaseHubStopFenceForFollowUp` path; children's fences stay invalid
  (generation bumped), matching Stop semantics.

## Live evidence (run-139670)

Two interrupt calls returned `{"status":"cancelling"}`; the run stayed
"running" and a new contract-planner leg spawned afterwards. Only
`agent-loop/stop` terminated it. §2 contract: ctx.Err() alone is
defense-in-depth, not the guard.

## Tests

`bug593_interrupt_fence_test.go` — red-first: interrupt writes
`Stopped=true` for the run; parent interrupt fences children too.
