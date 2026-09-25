# BUG-479: mux upsert does not insert an unseen lane into Desktop history/board

## Metadata

- Document ID: `BUG-479`
- Phase: `bugfix`
- Status: `done`
- Severity: `medium-high`
- Evidence: `code-confirmed; Desktop live verification pending`
- Feature Keys: `event-plane`, `project-nav`
- Parent Documents: `CP-84`, `CP-82`
- Affected Area: `apps/desktop-flowpilot/src/state/store.ts`

## Summary

The CP-84 mux carries every top-level active lane, including runs created after
the last 30-second history poll. `patchHistoryLane` only updates a row if
`projectHistoryById[projectId]` already contains that `runId`; otherwise it
returns without inserting. The attention queue synthesizes a row only for
attention derivation, not for the store consumed by Navigator/SessionsBoard.
An unseen active lane can therefore remain absent from the board until polling.

## Evidence

- `consumeRunUpdatesLoop` calls `patchHistoryLane` on completed snapshots and
  live upserts.
- `patchHistoryLane` returns `{}` when `items` is missing or no row matches.
- `SessionsBoard`, `Navigator`, `SpectatorPane` read
  `projectHistoryById`, not the private `muxLanes` map.
- Existing mux-loop test seeds a prior history row before asserting a status
  patch; no test covers a mux-only run.

## Expected vs Actual

- Expected: a newly projected top-level lane appears in the board immediately.
- Actual: only already-polled rows get realtime status; unseen lanes wait for
  the next history poll unless they also generate an attention item.

## Impact

The awareness layer can violate CP-84's <1s lane visibility target for newly
created background runs, especially runs created by another client/project.
Decision cards may still appear, masking the missing active lane case.

## Required Tests

- RED: mux snapshot containing a run absent from history inserts a minimal row.
- RED: live upsert for a new run inserts exactly once and later poll reconciles.
- Remove/terminal semantics do not delete useful completed history.
- Cross-project lane insertion does not change focused project/run.
- Desktop live: create run B from another client while focused on A.

## Implementation Plan

### P-1 — RED store-loop tests

- Extend `muxLoop.test.ts` with empty `projectHistoryById`, then feed a complete
  snapshot and a later upsert for a new run.
- Assert the current store omits the run from Navigator/SessionsBoard input even
  though `attentionQueue.muxLaneCount()` sees it.
- Cover selected and non-selected projects.

### P-2 — Authoritative minimal-row insertion

- Replace patch-only `patchHistoryLane` behavior with an upsert helper.
- When absent, synthesize a bounded `RunHistoryItem` from projection fields;
  when present, patch realtime-owned fields only.
- Preserve server-polled metadata not carried by mux; a later history poll
  merges/replaces the minimal row by `runId` without duplication.
- Do not switch `selectedProjectId`, `runId` or composer state.

### P-3 — Removal and terminal policy

- Define whether a mux remove drops only an ephemeral minimal row or retains a
  completed history row. Follow CP-84 D-6: completed useful history stays.
- Mark minimal rows so reconciliation can distinguish them without creating a
  second history registry.

### P-4 — Component and live proof

- Add component render coverage showing a mux-only row in SessionsBoard and
  Navigator.
- Live: client B creates a background run while client A stays focused; measure
  appearance under one second and verify no focus/draft change.

## Definition of Done

- [ ] Snapshot and upsert insert unseen top-level lanes immediately.
- [ ] Existing rows receive status/updatedAt patches without metadata loss.
- [ ] Poll reconciliation produces exactly one row per run.
- [ ] Terminal/remove preserves useful completed history per D-6.
- [ ] Cross-project insertion never steals focus or clears draft.
- [ ] SessionsBoard/Navigator component tests cover mux-only lane rendering.
- [ ] Live two-client measurement meets the <1s requirement.
- [ ] CP-82 board and CP-84 mux/cache suites remain green.
