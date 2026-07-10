# CA-273: Navigator Sync/Restore Batch Progress x/y

## Scope

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- `requirements/08-Task/done/Task-213-Desktop-Chat-Sync-Restore-Batch-Progress-Indicator.md`

Implements `Task-213`: Navigator's batch sync-to-Drive and batch restore-from-Drive actions now show `x/y` (done/total) progress instead of a single bare spinner.

## Completed

### I-1 — Shared sync-up batch progress in the store

Added `syncBatchProgress?: { projectId; done; total }` to the store and a new `syncRuns(runIds, projectId)` action that sets the counter at batch start, increments it as each item settles, and clears it when the batch ends. `syncAllInProject` now computes its target list and delegates to `syncRuns`.

### I-2 — Unified the two sync-up call sites

`Navigator.tsx`'s `executeConfirm` (selection-mode "Sync" confirm) now calls the shared `syncRuns` instead of looping `syncHistoryRun` itself, so the project "Sync all" chip and the selection-mode confirm both drive the same `syncBatchProgress` counter — one source of truth, consistent with `BUG-088`'s `V-1`/`V-2`.

### I-3 — Sync-up UI

The project-level "Sync all" chip renders `{done}/{total}` next to the spinner while `syncBatchProgress` matches the selected project.

### I-4 — Sync-down batch progress + UI

Added local `restoreProgress` state in `Navigator.tsx`, tracked inside `restoreAll()`. The "Restore all" chip and the selection-mode "Restore All" button now render `{done}/{total}` next to their spinner.

### I-5 — Fixed flicker between batch items

The chip's spinner/label visibility was initially gated by `isProjectSyncing` (true only while some row's `syncStatus === "syncing"` at that exact instant). Between one item finishing and the next starting, no row is momentarily "syncing," so the chip flickered back to its idle state and the `x/y` counter disappeared, then reappeared once the next item began. Fixed by gating on `Boolean(activeSyncProgress) || isProjectSyncing(...)` instead — `syncBatchProgress` spans the whole batch, so the spinner and counter now hold steady from `0/total` through `total/total`.

### I-6 — Regression test

Added `syncAllInProject reports x/y batch progress via syncRuns and clears it when the batch finishes` to `store.test.ts`, gating two mock sync calls to assert `{done:0,total:2}` at start, `{done:1,total:2}` mid-batch, and `undefined` after completion.

## Verification

- `npm --prefix apps/desktop-flowpilot run typecheck` → passed
- `npx tsx --test src/state/store.test.ts` → 83 pass / 3 fail (pre-existing, unrelated — confirmed identical with these changes stashed)
- `npx tsx --test src/components/navigatorHistory.test.ts` → 11/11 pass, unchanged
- Live browser verification not performed: the desktop app's bootstrap screen requires the local-runner backend on `127.0.0.1:4317`, out of scope to stand up for this UI-only slice

## Residual Notes

- Per-row (single-item) sync/restore spinners are unchanged — `x/y` only applies to batch actions.
- No runner/backend API changes; the counter is purely a client-side derivation over the existing one-item-at-a-time loop.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: Task-213
change_type: feature
summary: Navigator sync/restore batch x/y progress indicator
# --->8---
