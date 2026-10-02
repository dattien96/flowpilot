# CA-1130 — BUG-582: step timeline polluted by settled overlay rows

## Why

Live run-100368 showed ~14 step rows for a 9-node vibe sprint. While the
owner-debate overlay was mounted, `reseedFlowStepRuntime` seeded rows for the
debate graph (debate_trigger/owner_1/owner_2/debate_synthesis); on restore,
BUG-562's merge reseed kept them as "historical" rows, so they rendered in
the sprint timeline forever. Companion defect: `agent_activity` heartbeats
fell through the timeline reducer's default and re-appended a blank
"Thinking..." row every throttle tick.

## What changed

`apps/local-runner/internal/runner/interactive_handlers.go`
(`workflowStepsRuntime`):

- For flow-engine-driven runs with a tracked topology, the client projection
  now returns only rows whose NodeID is in the run's CURRENT topology —
  `activeFlowNodes ∪ vibeParkedNodes` (parked sprint nodes stay visible while
  an overlay is mounted; settled overlay rows drop out after restore). The
  durable rows and transition log are untouched — this narrows only the view.
  Rows with empty NodeID (catalog-seeded) and runs with no tracked topology
  fail open so replayed history stays complete.

`apps/desktop-flowpilot/src/state/timelineReducer.ts`:

- `agent_activity` gets an explicit no-op case — keeps an existing thinking
  row, never creates one and never pushes a timeline row.

## Tests

- Runner `bug582_step_projection_test.go`: settled overlay rows dropped
  post-restore; mounted overlay projects the parked+active union; runs with
  no tracked topology fail open.
- Desktop `store.bug576-live-loop-status.test.ts` asserts agent_activity adds
  no timeline row.
