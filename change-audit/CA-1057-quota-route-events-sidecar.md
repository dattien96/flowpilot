# CA-1057 — quota route decisions persist to the flow-event sidecar

## What changed

`local_file_session_store.go` — `isFlowSidecarEventType` now includes
`EventQuotaRouteCommitted`, `EventQuotaRouteStopped`, `EventQuotaRouteBlocked`
alongside the existing flow-context and user-attention types.

Quota route transitions are runner-emitted routing decisions with no provider
transcript representation. Without sidecar persistence they vanished from the
reconstructed timeline after a restart — observers replaying
`<runID>-flow-events.ndjson` lost every quota veto/repin/stop while session
rows still showed their downstream effects. Recorded as R.2 item 8 in
`requirements/07-Coding-Plan/todo/CP-Full-Live-Test.md`.

## Red → green

`internal/runner/quota_route_sidecar_test.go` (new):
`TestQuotaRouteEventsPersistToFlowSidecar` appends all three route types and
asserts `LoadFlowEvents` replays them — red before the change (0 events
returned), green after. The events already render through the live timeline
path (`quota_decision.go`), so replay requires no new projection.
