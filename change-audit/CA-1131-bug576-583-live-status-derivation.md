# CA-1131 — BUG-576 + BUG-583: stale Completed chip and status flapping

## Why

Live run-100368: the header chip read "Completed" while the backend sprint
was still running. Two mechanisms:

- `statusFromEvent` maps any `turn_completed` to `completed` — but on a
  flow-driven hub run a provider turn boundary is not the run's terminal;
  the sprint keeps driving via legs, and with no `agent_graph_updated`
  arriving during a long silent leg turn the chip stayed Completed.
- `statusFromEvent`'s default branch unstuck `waiting_*` → `running` on any
  event — so the new `agent_activity` heartbeat would flap a parked run's
  status every 3s.

## What changed

`apps/desktop-flowpilot/src/state/store.ts` (`applyEvent`):

- On `turn_completed`, when the event's run owns an open agent loop
  (`agentGraphSnapshot.parentRunId === workflowRunId` and loop status is not
  done/stopped), the run status is re-derived via
  `deriveOrchestrationRunStatus` — open loop reads running/blocked instead of
  Completed; a done loop still completes.

`apps/desktop-flowpilot/src/state/timelineReducer.ts` (`statusFromEvent`):

- `agent_activity` returns `prev` — a liveness stamp is never a state
  transition.

## Tests

- `store.bug576-live-loop-status.test.ts`: open loop + turn_completed stays
  running; done loop completes; no snapshot completes (plain chat
  unaffected); blocked loop reads blocked; agent_activity never flips a
  parked waiting status.
