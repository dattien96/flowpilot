# CA-1008 — BUG-506: child run creation falls back to canonical flowRef on catalog outage

## Context

Live run-20370: `spawnChildRun` passed the parent's `workflowID` — the
workflow-mirror row UUID — into `createRun`. During a Supabase catalog
outage the UUID couldn't resolve from `ListWorkflowSteps`, and the
embedded-pack fallback `flowStepsFromDefinition(uuid)` also failed
(pack lookup is by flow id, not mirror UUID), so child creation returned
`catalog_unavailable` and the flow dead-parked with no card.

## Changes

- `createRun` (`interactive_handlers.go`): when `flowStepsFromDefinition`
  fails on `in.WorkflowID` and `in.FlowRef` is a different non-empty
  value, retry resolution on `in.FlowRef` — the canonical pack reference
  that resolves from the embedded flow pack with no store access.
- `spawnChildRun` (`interactive_service.go`): captures the parent's
  `chatFlowRef` and passes it as `StartRunInput.FlowRef`, so delegate
  children inherit the outage-proof reference alongside the mirror UUID.

## Tests

`bug506_spawn_catalog_outage_test.go` (new, additive): direct createRun
with a mirror UUID + canonical FlowRef resolves during a simulated
catalog outage; a spawned child inherits the parent's canonical flowRef.

## Verification

`go test -count=1 -run TestBug506 ./internal/runner/` — green.
