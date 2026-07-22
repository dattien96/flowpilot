# CA-400: session_unavailable sync failures now persist as terminal, not retried forever

## Summary

Found alongside CA-399/BUG-310 in the same live investigation: 4 of the 9
Gate-sandbox runs stuck in the unsynced count were cancelled chats that
genuinely have no session file on this machine (`session_unavailable`,
`"session data not found on this machine"`). This is a real, permanent fact
about those specific runs, not a bug in reporting -- but nothing persisted it,
so every future "Sync all" batch silently re-attempted and re-failed them
forever, keeping the Navigator's unsynced-count above zero indefinitely.

## Root cause

`BuildChatSessionSyncManifest` (`chat_session_sync.go:317-319`) correctly
detects "chat-kind run, no transcript found anywhere" and returns
`session_unavailable`, but never wrote this fact to the persisted store. The
frontend's own `syncStatus: "failed"` + `unavailableReason` (set locally per
BUG-088's convention) only lives in the browser's in-memory state, and gets
overwritten back to blank by the Navigator's own periodic `loadRunHistory()`
poll (every 3-10s) re-fetching from the backend, which had nothing to report.
Full detail in
[BUG-311](../requirements/09-BugFix/done/BUG-311-Session-Unavailable-Sync-Never-Persists-Terminal-Status.md).

## Fix

The `session_unavailable` branch of `BuildChatSessionSyncManifest` now calls
`updateLocalSessionSyncStatus` to persist `SyncStatus: "unsyncable"` before
returning the error. `isSyncableRun` (`navigatorHistory.ts`) now excludes
`syncStatus === "unsyncable"` alongside the existing `"synced"` exclusion.
`"unsyncable"` is deliberately a new, distinct value from `"failed"`:
`"failed"` is still retried on the next attempt (unchanged); `"unsyncable"` is
a terminal state reserved for this one deterministically-permanent condition.

Scoped to exactly this one error path: no change to `syncChatRunToDrive`'s
success path, no change to how other failure codes
(`google_drive_not_connected`, transient `workflow_state_unavailable`, etc.)
are handled -- those remain retryable, unchanged.

## additive-tests-only compliance

New file only: `bug311_test.go`. New test case added to the existing
`navigatorHistory.test.ts` (new `test(...)` block; no existing assertion
edited). No existing Go test file touched.

## Verification

- `go build ./...`, `go vet ./internal/runner/`: clean.
- New test `TestBuildChatSessionSyncManifestPersistsUnsyncableOnMissingSession`:
  PASS on the fix (including a second-call stability check); confirmed FAIL on
  the pre-fix baseline via `git stash`.
- New test in `navigatorHistory.test.ts` ("isSyncableRun permanently excludes
  an unsyncable run"): PASS; full 12-test suite green.
- Existing `TestBuildChatSessionSyncManifestChatRunStillRejectsPlaceholderSession`
  (the pre-existing `session_unavailable` HTTP-behavior test): still PASS,
  confirming only the new persisted side effect was added, not a behavior
  change to the error response itself.
- Regression sweep: only the same 2 pre-existing, unrelated failures noted in
  CA-399, both confirmed present on baseline.
- Live re-verification: after restarting the dev stack, the 4 previously
  perpetually-failing cancelled runs in project "Gate-sandbox" now persist
  `syncStatus: "unsyncable"`.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: BUG-311
change_type: bugfix
summary: A chat run with no resumable session file now persists SyncStatus="unsyncable" on first sync attempt instead of being silently re-attempted and re-failed on every future batch, letting the Navigator's unsynced-count actually reach zero.
# --->8---
