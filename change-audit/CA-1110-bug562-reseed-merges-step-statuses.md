# CA-1110 — BUG-562: flow-step reseed merges statuses instead of wiping to PENDING

## Why

Live run-69320 (PrivateVault Task-024 sprint): every sub-flow resolve,
sprint mount, and debate settle re-seeds the run's step rows.
`reseedFlowStepRuntime` projected a fresh PENDING row set and replaced the
existing list wholesale — on the timeline every finished step flipped to
"pending" ("0/10"), and worse, the hub node (`synthesis`) reverted to
PENDING so CA-1087's stale-done guard consumed the next legitimate child
`done` as `flow_control_stale_hub_done` and never dispatched the successor.
The chain stranded; only a manual gate poke un-wedged it.

## What changed

`apps/local-runner/internal/runner/flow_step_runtime.go`:

- `reseedFlowStepRuntime` now seeds `mergeReseedSteps(...)` instead of the
  raw fresh projection.
- New `mergeReseedSteps(parentRunID, fresh)`: rows the flow already owns
  (non-empty `NodeID`) keep `Status`, `StartedAt`, `FinishedAt`,
  `RetryCount`, `RejectionNote`; node ids new to the resolved topology seed
  PENDING; prior flow-owned rows absent from the new node set are appended
  unchanged as historical rows. Catalog-seeded rows (empty `NodeID`, the
  workflow-picker shape) are still replaced wholesale on the first flow
  reseed — identity comes from NodeID, never list position.

## Invariant

Reseeding reconciles topology, not history: a DONE/RUNNING step survives a
re-resolve byte-for-byte until the engine itself transitions it.

## Tests

`bug562_reseed_preserves_status_test.go`:

- settled DONE/RUNNING rows keep status + timestamps + retry + note across
  a reseed; new nodes still seed PENDING
- sub-flow rows absent from the new topology persist as history
- first reseed over catalog rows (empty NodeID) still replaces wholesale
