# CA-1129 — BUG-584: live chip goes quiet during long leg turns

## Why

Live run-100368: the desktop subscribes only the focused (parent) run's SSE
stream. A child leg's events (message_delta, tool calls) ride the leg's own
stream, so a 20-minute leg turn produced zero parent-stream arrivals and the
"live — last event Xs ago" chip read `quiet` while the leg was actively
working.

## What changed

Runner (`apps/local-runner/internal/runner/`):

- `provider_event.go`: new broadcast-only `EventAgentActivity`
  (`agent_activity`) — a liveness stamp, never persisted (`persistEvent`
  skips it like `message_delta`), so history replay stays clean.
- `interactive_service.go`: `emitLocked` calls
  `forwardLegActivityStampLocked` for any child event; the helper walks the
  ancestor chain (depth-capped at 8) and emits a throttled `agent_activity`
  (3s per producing leg, `legActivityForwardInterval`) on each ancestor's
  stream carrying `ChildRunID`/`AgentName`.

Desktop (`apps/desktop-flowpilot/src/`):

- `types/contract.ts`: `agent_activity` added to the `ProviderEventDTO`
  union with optional `childRunId`/`agentName`.
- `state/runActivity.ts`: `stampRunActivity` credits BOTH the producing leg
  (`childRunId`) and the parent (`workflowRunId`) so the family rollup warms
  even if the leg's agentRuns row is stale.

## Tests

- Runner `bug584_leg_activity_forward_test.go`: child event forwards one
  stamp; throttle collapses bursts and re-arms after the window; root runs
  never self-forward; a grandchild warms the root stream.
- Desktop `runActivity.test.ts`: agent_activity stamps both lanes.
