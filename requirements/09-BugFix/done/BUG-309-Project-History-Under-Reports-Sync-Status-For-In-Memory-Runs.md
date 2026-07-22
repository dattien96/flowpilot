# BUG-309: Project History Under-Reports Sync Status For In-Memory Runs

## Metadata

- Document ID: `BUG-309`
- Title: `Project History Under-Reports Sync Status For In-Memory Runs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-22`
- Last Updated: `2026-07-22`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-060: Desktop Run History Empties After Switching Runs](../done/BUG-060-Desktop-Run-History-Empties-After-Switching-Runs.md) (introduced the in-memory / persisted-augment split in `projectRunHistory` this bug patches), [BUG-088: Desktop History Sync Actions Missing In-Progress Indicator](../done/BUG-088-Desktop-History-Sync-Actions-Missing-In-Progress-Indicator.md) (adjacent `syncStatus` UI surface, not touched by this fix), [Task-258: Per-Project Dispatch Log And Drive Sync](../../08-Task/done/Task-258-Per-Project-Dispatch-Log-And-Drive-Sync.md), [CA-397: Drive Sync Dispatch Log Redundant Reupload](../../change-audit/CA-397-drive-sync-dispatch-log-redundant-reupload.md) (adjacent Drive-sync perf bug found the same day, different root cause), [CA-398](../../change-audit/CA-398-project-history-in-memory-sync-status.md)
- Replaces: `none`
- Tags: `desktop, google-drive, chat-sync, run-history, regression, severity-medium`

## AI Quick View

### Summary

- The desktop Navigator's "Sync all" unsynced-count badge stayed stuck at a non-zero number (e.g. "15" of 17 chats) even right after a sync batch appeared to finish successfully.
- `projectRunHistory` builds each project's run list from two branches: an in-memory branch (`s.runs`, for runs still resident from the current process's lifetime) and a persisted-store augment branch (for runs that fell out of memory, e.g. after a restart).
- The in-memory branch's `runHistoryItem` literal never set `SyncStatus`, `SourceMachineID`, or `SourceRunID` — those fields are written straight to the persisted store by `updateLocalSessionSyncStatus`, asynchronously, well after a run's own turn lifecycle ends, and `interactiveRun` never carried them at all.
- Practical effect: any run created during the current server process's uptime (i.e. nearly all of them, since the runner had been running continuously) always reported `syncStatus=""` from this endpoint, regardless of how many times it had actually been synced to Drive, so the frontend's `isSyncableRun` filter kept counting it as unsynced forever.

### Current Ask

- Fixed. `projectRunHistory` now looks up each in-memory run's persisted `ProviderSessionState` (via the same `SessionHistoryReader.ListProviderSessionsByProject` already used by the persisted-augment branch) and copies `SyncStatus`/`SourceMachineID`/`SourceRunID` onto the in-memory-branch item when a persisted record exists.

### Key Decisions

- `V-1` The in-memory branch must report the same Drive-sync fields as the persisted-augment branch for the same run, since both branches feed the identical `runHistoryItem` shape the frontend reads through one `isSyncableRun` filter.
- `V-2` Source the fields from the persisted store rather than adding them to `interactiveRun` itself — sync status is set well after a turn completes, by a separate user-triggered action, and the persisted store (not the live run object) is already the accepted authority for it (per BUG-060 `V-2`).
- `V-3` Fetch `ListProviderSessionsByProject` once, build a `runID -> ProviderSessionState` lookup, and only use it to backfill the 3 sync fields on the in-memory branch; leave the existing persisted-augment branch's own fetch/logic untouched to keep the fix minimal and low-risk.

### Constraints

- Scoped to `projectRunHistory` only. No change to how/when `SyncStatus` is written (`updateLocalSessionSyncStatus`, `syncChatRunToDrive`), no change to the frontend `isSyncableRun`/Navigator badge logic, no change to the persisted-augment branch.
- If `s.workflowStore` does not implement `SessionHistoryReader` (some test doubles), the lookup map stays empty and the in-memory branch behaves exactly as before the fix — no new failure mode introduced.

### Open Questions

- None. The desktop-side "looks synced but count doesn't drop" symptom noted as unexplained in [CA-397](../../change-audit/CA-397-drive-sync-dispatch-log-redundant-reupload.md)'s "Not in scope" section is now root-caused by this bug; whether it fully explains every instance of that symptom (versus, in part, the CA-397 slowness itself) was not separately re-tested live after this fix, since the root cause here is proven directly via the unit test and via a live data pull, not by re-running the original slow 17-chat batch end-to-end.

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go` — `projectRunHistory` (in-memory branch), `runHistoryItem` struct.
- `apps/local-runner/internal/runner/bug309_test.go` — `TestProjectHistoryReportsSyncStatusForInMemoryRun`.
- Live evidence: `GET /client/projects/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/workflow-runs` against the running dev server (project "Gate-sandbox") returned 17 runs, 15 with no `syncStatus` key and only 2 (both `runKind=workflow`, both already fallen out of the in-memory map) with `syncStatus="synced"`.

## 1. Issue Summary

On the desktop app, the Navigator's per-project "Sync all" chip shows a count of chats still needing a Drive sync. After running a full sync batch that appeared to complete (spinner finished, no visible errors), the count did not drop to reflect the chats that were actually just synced — e.g. it kept showing "15" out of "17 chats" both before and after clicking sync again.

## 2. Parent Links

- impacted coding plan: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) (discovered during CP-51's C6 Drive-sync live verification)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop-flowpilot (Electron renderer) against the local Go runner (`apps/local-runner`), any project with chat runs created in the current runner process's uptime and synced to Drive at least once.
- reproduction steps:
  1. Start or resume several chat runs in a project (they now live in `InteractiveService.s.runs`).
  2. Sync one or more of them to Drive (`POST /client/workflow-runs/{runId}/sync`), succeeding.
  3. Without restarting the runner, call `GET /client/projects/{projectId}/workflow-runs` (or open the Navigator again).
  4. The just-synced run's `syncStatus` comes back empty/missing, not `"synced"`.
- frequency: 100% for any run still resident in `s.runs` — i.e. effectively always, for a runner that has not been restarted since the run was created.

## 4. Expected vs Actual

- expected: `GET /client/projects/{projectId}/workflow-runs` reports the true, persisted `syncStatus` (and `sourceMachineId`/`sourceRunId`) for every run, whether or not that run is still resident in the in-memory `s.runs` map.
- actual: runs still in `s.runs` always reported `syncStatus=""` (key omitted, `json:"syncStatus,omitempty"`), regardless of what was actually persisted; only runs that had fallen out of memory (persisted-augment branch) reported the real value.

## 5. Impact

- users affected: any desktop user syncing chat history to Drive in a long-lived runner session (i.e. normal usage — most users do not restart the runner between syncs).
- workflows affected: the Navigator "Sync all" unsynced-count badge and button label; `isSyncableRun`'s local-`syncStatus` check (frontend behavior itself was correct — it was fed wrong data); CP-51 C6 live-verification evidence quality (the "looks synced but count stuck" symptom recorded as an open gap in CA-397).
- severity: medium — no data loss (Drive uploads themselves succeed; `dispatch.ndjson`/session files are correctly written), but the UI's sync-progress signal was actively misleading, undermining user trust that sync had worked and causing repeated unnecessary "Sync all" clicks.

## 6. Root Cause

- hypothesis: cancelled runs were silently and permanently failing to sync (`session_unavailable` when a cancelled turn has no resolvable provider transcript), and failed rows are never excluded by `isSyncableRun` (which only excludes `syncStatus === "synced"`), so a persistently-failing subset would explain a badge that never reaches zero.
- confirmed cause: a live data pull from the running dev server (`GET /client/projects/{id}/workflow-runs` for project "Gate-sandbox") showed the pattern did not correlate with run status (`cancelled` vs `completed`) at all — 15 of 17 runs of every status showed no `syncStatus`, and only the 2 runs that had fallen out of the in-memory map (`runKind=workflow`, from an earlier process lifetime) showed `syncStatus="synced"`. Reading `projectRunHistory` (`interactive_handlers.go:957`) confirmed the actual cause: its in-memory branch (for runs in `s.runs`) built `runHistoryItem` without `SyncStatus`/`SourceMachineID`/`SourceRunID` at all, while its persisted-augment branch (for runs no longer in `s.runs`, added by BUG-060 F-1) correctly copied all three from the persisted `ProviderSessionState`. `interactiveRun` (the `s.runs` value type) has no sync-related fields — `updateLocalSessionSyncStatus` writes sync state directly to the persisted store, never back into the live run object, so the in-memory branch had no source for these fields at all until this fix added the lookup.
- evidence: `TestProjectHistoryReportsSyncStatusForInMemoryRun` fails on the pre-fix code with exactly the predicted symptom (`syncStatus="", want "synced"`), confirmed by temporarily stashing the production fix and re-running the test against the unmodified baseline.

## 7. Fix Strategy

- `F-1` In `projectRunHistory`, fetch `ListProviderSessionsByProject` once up front (when `s.workflowStore` implements `SessionHistoryReader`) and build a `runID -> ProviderSessionState` map.
- `F-2` When building each in-memory-branch `runHistoryItem`, look up the run's id in that map and copy `SourceMachineID`/`SourceRunID`/`SyncStatus` onto the item if a persisted record exists.
- `F-3` Leave the existing persisted-augment branch untouched (it already sourced these fields correctly) to keep the change minimal and scoped to the confirmed defect.

## 8. Validation

- `V-1` New test `TestProjectHistoryReportsSyncStatusForInMemoryRun` (`bug309_test.go`) — PASS on the fix; confirmed FAIL on the pre-fix baseline via `git stash` (same technique used for CA-397's pre-existing-failure proof), with the exact predicted symptom.
- `V-2` Existing `TestRunHistoryEmptiesAfterServiceRecreation` (BUG-060) — still PASS; the persisted-augment branch and the BUG-060 rehydration behavior are unaffected.
- `V-3` `go build ./...` and `go vet ./internal/runner/` — clean.
- `V-4` Regression sweep (`TestDispatch*`, `TestCrashMatrix*`, `TestRecoveryScanner*`, `TestSync*`, `TestChatSession*`, `TestRestoreChatRun*`, `TestDeleteRun*`, `TestTurn*`, `TestInteractive*`, `TestBug*`, `TestRunHistory*`, `TestProjectHistory*`): only the same pre-existing, already-documented failure (`TestRestoreChatRunFromDriveMissingActiveAccountHome`, see CA-397) — no new failures.
- `V-5` Live evidence: the failure mode was independently confirmed against the running dev server's real data (project "Gate-sandbox", 17 runs, 15 missing `syncStatus`) before writing the fix, not just inferred from code reading.

## 9. Regression Guard

- tests: `apps/local-runner/internal/runner/bug309_test.go` (`TestProjectHistoryReportsSyncStatusForInMemoryRun`), plus the existing `TestRunHistoryEmptiesAfterServiceRecreation` (BUG-060) as a non-regression check on the sibling branch.
- alerts: none.
- audit checks: [CA-398](../../change-audit/CA-398-project-history-in-memory-sync-status.md).

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a bug in an existing read path, not a change to business intent, acceptance criteria, or the `SS-11`/`SD-12` design (which already specified the persisted store as the sync-status authority; the in-memory branch simply never consulted it).
- notes left unchanged on purpose: the desktop Navigator UI (`Navigator.tsx`, `navigatorHistory.ts`) was not touched — its `isSyncableRun`/badge logic was already correct and required no change once fed correct data.
