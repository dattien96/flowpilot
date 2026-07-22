# CA-398: Project history now reports real sync status for in-memory runs

## Summary

Found while explaining the CA-397 Drive-sync fix to the user: the desktop
Navigator's "Sync all" unsynced-count badge stayed stuck at a non-zero number
(observed live: "15" of 17 chats in project "Gate-sandbox") even right after a
sync batch appeared to finish. A live data pull from the running dev server
(`GET /client/projects/{id}/workflow-runs`) confirmed 15 of 17 runs had no
`syncStatus` at all, uncorrelated with run status (`cancelled`/`completed`) —
only the 2 runs that had fallen out of the in-memory run map reported
`syncStatus="synced"`.

## Root cause

`projectRunHistory` (`interactive_handlers.go:957`) builds each project's run
list from two branches: an in-memory branch (`s.runs`, for runs still
resident from the current process's lifetime) and a persisted-store augment
branch (`s.workflowStore`, for runs that fell out of memory — added by
BUG-060 F-1). The in-memory branch's `runHistoryItem` literal never set
`SyncStatus`/`SourceMachineID`/`SourceRunID` — `interactiveRun` (the `s.runs`
value type) never carried those fields at all, since Drive-sync status is
written straight to the persisted store by `updateLocalSessionSyncStatus`,
asynchronously, well after a run's own turn lifecycle ends. Any run created
during the current runner process's uptime therefore always reported
`syncStatus=""` from this endpoint, regardless of how many times it had
actually been synced — full root cause and evidence in
[BUG-309](../requirements/09-BugFix/done/BUG-309-Project-History-Under-Reports-Sync-Status-For-In-Memory-Runs.md).

## Fix

`projectRunHistory` now fetches `ListProviderSessionsByProject` once up front
(the same call the persisted-augment branch already made) and builds a
`runID -> ProviderSessionState` lookup. When building each in-memory-branch
item, it copies `SourceMachineID`/`SourceRunID`/`SyncStatus` from that lookup
if a persisted record exists for that run id. The persisted-augment branch
itself was left untouched — it already sourced these fields correctly.

Scoped to exactly the confirmed defect: no change to when/how `SyncStatus` is
written, no change to the frontend `isSyncableRun`/Navigator badge logic
(already correct — it was being fed wrong data), no change to the
persisted-augment branch's own logic.

## additive-tests-only compliance

New file only: `bug309_test.go` (matches the existing per-bug test file
convention, e.g. `bug060_test.go`). No existing test file touched.

## Verification

- `go build ./...` and `go vet ./internal/runner/`: clean.
- New test `TestProjectHistoryReportsSyncStatusForInMemoryRun`: PASS on the
  fix. Confirmed it correctly FAILS on the pre-fix baseline (via `git stash`
  on just the production file) with the exact predicted symptom:
  `syncStatus="", want "synced"`.
- Existing `TestRunHistoryEmptiesAfterServiceRecreation` (BUG-060): still
  PASS — the persisted-augment branch and restart-rehydration behavior are
  unaffected.
- Regression sweep (`TestDispatch*`, `TestCrashMatrix*`, `TestRecoveryScanner*`,
  `TestSync*`, `TestChatSession*`, `TestRestoreChatRun*`, `TestDeleteRun*`,
  `TestTurn*`, `TestInteractive*`, `TestBug*`, `TestRunHistory*`,
  `TestProjectHistory*`): only the same pre-existing, already-documented
  failure (`TestRestoreChatRunFromDriveMissingActiveAccountHome`, see
  CA-397) — no new failures.
- Live evidence gathered *before* writing the fix (not just inferred from
  code reading): the failure pattern was independently confirmed against the
  running dev server's real project data.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: BUG-309
change_type: bugfix
summary: projectRunHistory's in-memory branch now reports real SyncStatus/SourceMachineID/SourceRunID instead of always blank, fixing a Navigator unsynced-count badge that never reached zero.
# --->8---
