# CA-1120 — BUG-577: sprint boundary inherits DONE statuses from prior sprint

## Why

Live run-100368: after Task-025's sprint crossed the boundary into Task-026,
the same `vibe-sprint` topology was re-resolved — but downstream step rows
(`coder`/`validate`/`reviewer`/`audit`) still carried DONE from the previous
sprint. The walker skipped every node that had stale DONE, so Task-026 could
auto-finalize an audit on scaffold stubs alone — exactly the false-green the
gate chain exists to prevent. Observed live: sprint-2 showed `coder..audit`
DONE while only tdd stubs existed on disk.

## What changed

`apps/local-runner/internal/runner/flow_executor.go`:

- When the resolved topology is `vibe-sprint` and a new sprint is starting,
  the flow step runtime is reseeded and **every** vibe sprint node is reset
  to PENDING before forward predecessors of the chosen start node are marked
  SKIPPED. Skipped stays a boundary-only annotation; DONE never survives
  across a sprint boundary.

## Invariant

A sprint boundary is a fresh execution of the same topology — step statuses
are per-sprint evidence and must not be inherited by the next task.

## Tests

`bug577_sprint_boundary_reseed_test.go` (red → green):

- `TestBUG577BoundaryStartReseedsDownstreamSteps` — starting the next sprint
  resets all sprint node statuses to PENDING before predecessor SKIPs apply
