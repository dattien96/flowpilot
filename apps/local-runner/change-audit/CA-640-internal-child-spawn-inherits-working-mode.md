# CA-640 — Internal Child Spawn Inherits Parent Working Mode (BUG-547)

**Date**: 2026-09-28
**Author**: Devin
**Ticket**: BUG-547

---

## Problem

Live `run-39478` (`:4322`): a `vibe-cp-ingest` parent started correctly with
`workingMode: vibe` + `X-Client: tui`, resolved its entry step, then died at
its first child spawn:

```
working_mode_flow_forbidden: working_mode_flow_forbidden
```

Root cause: `spawnChildRun` carried the parent's `WorkflowID` (flow lineage
for step resolution) into `StartRunInput` but did **not** carry
`WorkingMode`. `createRun` normalized the empty mode to `dev`, and
`enforceWorkingModeStart` then treated the inherited workflowID as a
user-selected flow mount — `vibe-cp-ingest` is a vibe-family flow, so the
dev-mode child start was rejected fail-closed. Every vibe flow was a
dead-end at its first internal spawn; the red test reproduced the same gate
error on `spawnChildRun` directly.

The inherited workflowID is lineage/metadata, not a fresh user mount: the
parent already passed the start-family and client gates, and the child is
created by the engine on the parent's behalf.

## Fix

`StartRunInput` gained an internal-only marker:

```go
SpawnedInternally bool `json:"-"`
```

- `spawnChildRun` sets `SpawnedInternally: true` and
  `WorkingMode: parentWorkingMode` (captured from `parentRun.workingMode`).
- `enforceWorkingModeStart` still normalizes the mode first (fail-closed on
  garbage), then returns early for internal spawns — skipping only the
  user-mount gates (`CheckClient`, pinned-run, flow-family). All gates still
  apply to ordinary `POST /client/workflow-runs` starts and turn-level
  flowRef mounts.
- `json:"-"` means the flag can never be supplied on the wire — there is no
  user-controlled bypass.

## Files

- `internal/runner/provider_event.go` — `StartRunInput.SpawnedInternally`
  field + contract comment.
- `internal/runner/working_mode_http.go` — `enforceWorkingModeStart`:
  normalize → early return for internal spawns → unchanged user gates.
- `internal/runner/interactive_service.go` — `spawnChildRun`: capture parent
  mode, set `SpawnedInternally` + `WorkingMode` on the child input.
- `internal/runner/bug547_vibe_child_spawn_mode_test.go` — red→green test.

## Test evidence

- `TestBug547_VibeParentChildSpawnNotFlowGated` — red before the fix
  (`working_mode_flow_forbidden`), green after; asserts the child inherits
  `vibe` mode.
- Live `:4322` post-fix binary: `run-40835` (`vibe-cp-ingest`, vibe+tui)
  spawned child `run-40840` (`vibe-intake`) at 13:43:32 with
  `blockedStart=false` and zero `working_mode_flow_forbidden` in the server
  log — the exact call that dead-ended `run-39478` on the old binary.

## Risk / blast radius

- Internal spawn path only: `spawnChildRun` (flow delegates, cohort members,
  route successors via `SpawnAgentInput` → `StartRunInput`).
- User start gates unchanged: dev-mode `working_mode_flow_forbidden` and
  vibe `client_forbidden` still fail closed on `handleStartRun` /
  turn-level flowRef mounts (BUG-400 guard intact).
- Mode normalization still runs for internal children — a corrupt parent
  mode fails closed, never silently defaults.
