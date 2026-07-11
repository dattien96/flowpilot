---
name: Task-219-Desktop-Chat-Sync-Restore-Batch-Progress-Indicator
description: Replace the single spinner on Navigator's batch sync-to-Drive and batch restore-from-Drive actions with an x/y (done/total) progress indicator.
metadata:
  type: task
---

## Metadata

- Document ID: `Task-219`
- Title: Desktop Chat Sync/Restore Batch Progress Indicator
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `—`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-33: Desktop Project Chat Drive Folder Selection](../../07-Coding-Plan/done/CP-33-Desktop-Project-Chat-Drive-Folder-Selection.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Child Documents: `—`
- Related Documents: `BUG-088-Desktop-History-Sync-Actions-Missing-In-Progress-Indicator.md`, `CA-103-navigator-sync-progress-indicator.md`, `Task-190-Sync-And-Restore-Flow-Engine-Runs-To-Drive.md`
- Replaces: `—`
- Tags: `desktop, navigator, chat-sync, ui`

## AI Quick View

### Summary

- Navigator's batch sync-to-Drive ("Sync all" chip, selection-mode "Sync" confirm) and batch restore-from-Drive ("Restore all" chip, selection-mode "Restore All") actions only ever showed a single bare spinner, with no sense of progress across a multi-item batch.
- Added an `x/y` (done/total) counter rendered next to the existing spinner on both directions.
- Sync-up: unified the two batch call sites onto one new store action, `syncRuns(runIds, projectId)`, which owns a single `syncBatchProgress` counter.
- Sync-down: added local `restoreProgress` state in `Navigator.tsx`, tracked inside the existing `restoreAll()` loop.

### Current Ask

- Show `x/y` progress instead of a single loading icon for both sync-up (upload to Drive) and sync-down (restore from Drive) batch actions in the Navigator sidebar.

### Key Decisions

- `T-1` Keep the existing row-level `syncStatus`-driven spinner-vs-icon toggle unchanged; add a new, separate `x/y` counter only for batch total/done tracking.
- `T-2` Unify the two sync-up batch call sites (project "Sync all" chip and selection-mode "Sync" confirm) onto one store action, `syncRuns`, so there is exactly one source of truth for sync batch progress (`syncBatchProgress`) — consistent with `BUG-088`'s `V-1`/`V-2` decision not to introduce a second progress source of truth.
- `T-3` Sync-down progress (`restoreProgress`) stays component-local state in `Navigator.tsx`, matching where `restoreAll` already lived (`BUG-088` did not touch the restore path).
- `T-4` Per-row (single-item) sync/restore spinners are left untouched — `x/y` only makes sense where more than one item can be in flight.
- `T-5` The sync-up chip's spinner/label visibility is keyed off the batch (`syncBatchProgress` being present for the selected project), not off `isProjectSyncing`'s per-row "is some row syncing right now" snapshot — the latter flips false in the gap between one item finishing and the next starting, which caused the counter to visibly disappear and reappear between every item instead of holding steady until `total/total`.

### Constraints

- Keep the change inside the desktop Navigator UI and its store batch-sync plumbing; no runner/API changes.
- Do not regress the existing row `syncStatus` lifecycle used by retry/failure states (`BUG-088`).
- Do not regress the existing sequential, one-item-at-a-time throttling that avoids hammering Drive.

### Open Questions

- None.

### Source Refs

- `CP-33`, `SD-14`, `BUG-088` (`V-1`, `V-2`), `CA-103`

## 1. Goal

Give both chat-sync directions (up: sync-to-Drive, down: restore-from-Drive) a visible `x/y` (done/total) progress counter on their batch actions in the Navigator sidebar, replacing the current single spinner-only feedback.

## 2. Parent Links

- coding plan: `CP-33-Desktop-Project-Chat-Drive-Folder-Selection.md`
- tech design: `SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md`
- system spec: `SS-11-Workflow-With_Session.md`
- specific upstream ids: `BUG-088` (`V-1`, `V-2`), `CA-103`

## 3. Trigger

User-reported UX gap: the Navigator's sync-up and sync-down batch actions only show one loading icon regardless of how many chats are being processed, giving no sense of progress on multi-item batches. Requested an `x/y` (done/total) indicator on those batch actions.

## 4. Exact Change

- `T-1` `apps/desktop-flowpilot/src/state/store.ts`: added `syncBatchProgress?: { projectId; done; total }` state and a new `syncRuns(runIds, projectId)` action that sets the counter at batch start, increments it after each item settles, and clears it when the batch finishes; `syncAllInProject` now computes its target run-id list and delegates to `syncRuns`.
- `T-2` `apps/desktop-flowpilot/src/components/Navigator.tsx`: `executeConfirm`'s selection-mode "Sync" branch now calls the shared `syncRuns` instead of looping `syncHistoryRun` itself, so both sync-up entry points drive the same counter.
- `T-3` `apps/desktop-flowpilot/src/components/Navigator.tsx`: the project-level "Sync all" chip now renders `{done}/{total}` next to the spinner while `syncBatchProgress` matches the selected project (falls back to "Syncing…" if `projectSyncing` is true but no progress snapshot is present yet).
- `T-4` `apps/desktop-flowpilot/src/components/Navigator.tsx`: added local `restoreProgress` state, set/incremented/cleared inside `restoreAll()`; the "Restore all" chip and the selection-mode "Restore All" button now render `{done}/{total}` next to their spinner.
- `T-6` `apps/desktop-flowpilot/src/state/store.test.ts`: added a regression test asserting `syncBatchProgress` reports `{done:0,total:2}` at batch start, `{done:1,total:2}` after the first item resolves, and clears to `undefined` once the batch completes.
- `T-7` `apps/desktop-flowpilot/src/components/Navigator.tsx`: `projectSyncing` is now `Boolean(activeSyncProgress) || isProjectSyncing(...)` instead of just `isProjectSyncing(...)`, so the chip's spinner + `x/y` label stay visible for the whole batch instead of flickering to the idle state between items.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/components/Navigator.tsx`, `apps/desktop-flowpilot/src/state/store.test.ts`
- modules: Navigator sidebar (History / Remote Chats sections), chat-sync store slice
- routes: `—`
- tables: `—`

## 6. Acceptance Check

- `npm --prefix apps/desktop-flowpilot run typecheck` → passed
- `npx tsx --test src/state/store.test.ts` (from `apps/desktop-flowpilot`) → 83 pass / 3 fail; the 3 failures (`selectProject resets the active chat run...`, `openHistoryRun does not set historyOpenError...`, `sendPrompt aborts an open-ended history replay stream...`) were confirmed pre-existing and unrelated by re-running the same suite with this task's changes stashed.
- `npx tsx --test src/components/navigatorHistory.test.ts` → 11/11 pass, unchanged.
- New test `syncAllInProject reports x/y batch progress via syncRuns and clears it when the batch finishes` passes.
- Manually caught during self-review: the initial `T-3` implementation kept using row-derived `isProjectSyncing` to gate the chip's spinner/label, which flickered to the idle state between items (see `T-5`/`T-7`); fixed by keying the gate off `syncBatchProgress` instead, which spans the whole batch.
- Manual browser verification **not performed**: the desktop app's bootstrap screen polls the local-runner Go backend on `127.0.0.1:4317` (`/health`, `/supabase-config`) before rendering the Navigator at all; standing that backend up was out of scope for this Navigator-only UI slice. Verification relies on the unit tests above instead of a live screenshot.

## 7. Out of Scope

- Per-row (single-item) sync/restore spinners — `x/y` only applies to batch actions where more than one item can be in flight.
- Any runner/backend batch-sync endpoint or manifest/progress API — this stays a client-side counter derived over the existing one-call-per-item loop.
- Fixing `CP-33`'s `Status: draft` metadata inconsistency despite the file living in `07-Coding-Plan/done/` (noted during research, not touched here).
- Parallelizing the sync/restore loops — they remain intentionally sequential (one at a time) per `BUG-088` and existing in-code comments, to avoid hammering Drive.

## 8. Completion Notes

- result: Implemented and unit-verified. `x/y` progress now renders for sync-up (project "Sync all" chip + selection-mode "Sync" confirm) and sync-down ("Restore all" chip + selection-mode "Restore All" button).
- follow-ups: Consider a lightweight way to stub the local-runner health/config endpoints for browser preview, so future Navigator UI changes can get a live screenshot instead of unit-test-only verification.
- upstream docs updated: none — this narrows/extends `BUG-088`'s existing "show progress" intent (a boolean spinner) into a counted `x/y` variant, without changing any acceptance criteria in `CP-33` or `SD-14`.
