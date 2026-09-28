# BUG-547 — internal child spawn loses parent working mode → vibe flows dead-end at entry

## Status
FIXED — unit-verified (red→green), live leg verified 2026-09-28
(run-40835/run-40840 on :4322 fp-live5).

## Live-found during
R17: launching `vibe-cp-ingest` on :4322 (2026-09-28).

## Live sequence
1. `POST /client/workflow-runs` `vibe-cp-ingest` + turn → parent
   `run-40835` mounted as vibe.
2. Entry node `vibe-intake` spawned its first child → child `createRun`
   rejected: `working_mode_flow_forbidden`.
3. `spawnChildRun` inherited the parent `WorkflowID` (vibe-family flow)
   but NOT `WorkingMode`; the child defaulted to `dev` and the
   user-mount working-mode gate vetoed every vibe-family spawn — every
   vibe flow dead-ended at its first internal child.

## Root cause
Internal spawn ran through the user-mount admission gate. There was no
marker distinguishing "the engine is continuing an already-admitted run"
from "a client mounted a fresh run", so the gate applied dev-mode rules
to a child that legitimately belonged to a vibe parent.

## Fix (CA-640)
- `StartRunInput.SpawnedInternally` marks engine-internal spawns.
- `spawnChildRun` propagates `WorkingMode` from the parent and sets the
  flag.
- `createRun` skips the user-mount/client gates for internal children
  while still normalizing + stamping the inherited mode — user-facing
  gates (`FlowAllowedForWorkingMode`, `CheckClient`) unchanged for real
  mounts.

## Tests
`internal/runner/bug547_internal_spawn_working_mode_test.go` (red→green):
vibe parent spawnChildRun no longer returns `working_mode_flow_forbidden`
and the child record carries `working_mode=vibe`.

## Live verification
Post-fix binary: `run-40835` (vibe parent) spawned `run-40840`
(vibe-intake/cp_reader) with `blockedStart=false`; zero
`working_mode_flow_forbidden` in the server log; flow advanced
cp_reader → cp_validator → cp_lock (parked) → resolved lock →
`suggest-requirement-change` pivot → cp_validator re-park — the flow
proceeds through its real entry path.
