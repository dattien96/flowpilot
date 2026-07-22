# BUG-311: Session Unavailable Sync Never Persists Terminal Status

## Metadata

- Document ID: `BUG-311`
- Title: `Session Unavailable Sync Never Persists Terminal Status`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-22`
- Last Updated: `2026-07-22`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-309: Project History Under-Reports Sync Status For In-Memory Runs](../done/BUG-309-Project-History-Under-Reports-Sync-Status-For-In-Memory-Runs.md), [BUG-310: Grok Drive Sync Always Fails Reading Session Directory As File](../done/BUG-310-Grok-Drive-Sync-Always-Fails-Reading-Session-Directory-As-File.md) (found and fixed alongside this bug, same live investigation), [BUG-088: Desktop History Sync Actions Missing In-Progress Indicator](../done/BUG-088-Desktop-History-Sync-Actions-Missing-In-Progress-Indicator.md) (established the local `syncStatus`/`unavailableReason` failure-display convention this bug extends), [CA-400](../../change-audit/CA-400-session-unavailable-persists-unsyncable.md)
- Replaces: `none`
- Tags: `desktop, google-drive, chat-sync, project-nav, regression, severity-medium`

## AI Quick View

### Summary

- A chat run cancelled/interrupted before its provider ever wrote a resumable session file returns `session_unavailable` on every sync attempt -- this is a permanent, deterministic fact about that specific run (it will never later gain a session file).
- The backend never persisted this fact anywhere. Only the frontend locally remembered "this failed" (`syncStatus: "failed"`, `unavailableReason`), and that local memory gets wiped by the Navigator's own periodic `loadRunHistory()` poll (every 3-10s) re-fetching from the backend, which still reported blank `syncStatus`.
- Net effect: this class of run was silently re-attempted and re-failed forever, on every "Sync all" batch, permanently keeping the Navigator's unsynced-count badge above zero for any project containing one.

### Current Ask

- Fixed. `BuildChatSessionSyncManifest` now persists `SyncStatus: "unsyncable"` the moment it determines a chat run has no transcript anywhere (the exact condition that produces `session_unavailable`); `isSyncableRun` (frontend) now excludes that status permanently, alongside the existing `"synced"` exclusion.

### Key Decisions

- `V-1` Only the specific "chat-kind run, no transcript found, `session_unavailable`" condition is treated as permanent. Other failure codes (`google_drive_not_connected`, transient `workflow_state_unavailable`, etc.) are left exactly as before -- still retried on the next attempt -- since those are not deterministically permanent facts about the run.
- `V-2` Reused the existing `SyncStatus` string field (no schema change) with a new value, rather than adding a separate boolean/flag column -- consistent with how `"synced"`/`"syncing"`/`"failed"` are already just string values on the same field.
- `V-3` `"unsyncable"` is intentionally distinct from `"failed"`: `"failed"` (existing, BUG-088) is expected to be retried; `"unsyncable"` (new) is a terminal state that must never be retried.

### Constraints

- Backend-only change to the sync-manifest build path plus a one-line frontend filter extension; no change to `syncChatRunToDrive`'s success path, no change to `syncHistoryRun`'s existing `"failed"`/`unavailableReason` handling for other error codes, no change to BUG-309's `projectRunHistory` in-memory-branch lookup (it already copies whatever `SyncStatus` string is persisted, so `"unsyncable"` flows through it unchanged).

### Open Questions

- None for the persistence/retry-loop defect itself. Whether the Navigator should show a distinct visual affordance for "unsyncable" rows (vs. simply no longer offering a sync action) is a UX polish question, not required to fix the confirmed retry-forever defect.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` -- `BuildChatSessionSyncManifest` (`session_unavailable` branch).
- `apps/desktop-flowpilot/src/components/navigatorHistory.ts` -- `isSyncableRun`.
- `apps/local-runner/internal/runner/bug311_test.go` -- `TestBuildChatSessionSyncManifestPersistsUnsyncableOnMissingSession`.
- `apps/desktop-flowpilot/src/components/navigatorHistory.test.ts` -- `"isSyncableRun permanently excludes an unsyncable run (BUG-311)"`.
- Live evidence: `GET /client/projects/{id}/workflow-runs` against the running dev server (project "Gate-sandbox") showed 4 cancelled runs (`run-35063`, `run-34555`, `run-34067`, `run-33581`) with persisted `syncStatus: "unsyncable"` after re-sync attempts post-fix, versus blank pre-fix.

## 1. Issue Summary

Any chat run that was cancelled early enough that its provider never wrote a resumable session file could never be synced -- correctly so, since there is nothing to sync -- but every "Sync all" batch re-attempted and re-failed it anyway, silently, forever. This meant a project containing even one such run could never show a fully-synced state.

## 2. Parent Links

- impacted coding plan: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) (discovered during the same CP-51 C6 live-verification session as BUG-309/BUG-310)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: `none known`

## 3. Environment and Reproduction

- environment: local-runner, any project containing a chat run that was cancelled before the provider wrote a session file.
- reproduction steps:
  1. Cancel a chat turn early enough that the provider CLI never persisted a resumable session file (e.g. `chat_history.jsonl` for Grok, a rollout file for Codex, a project transcript for Claude).
  2. `POST /client/workflow-runs/{runId}/sync-chat` -- fails with `session_unavailable`.
  3. Repeat the same call (or click "Sync all" again later) -- fails identically, every time, forever.
- frequency: 100% for any run matching this state.

## 4. Expected vs Actual

- expected: after the first `session_unavailable` failure, this run stops being offered for sync (no local data will ever exist for it), and the Navigator's unsynced-count reflects that.
- actual: the run was retried on every subsequent "Sync all" batch, failing identically every time, with the count never able to reach zero.

## 5. Impact

- users affected: any user with at least one cancelled-early chat in a project they try to fully sync.
- workflows affected: Navigator "Sync all" unsynced-count badge, per-row sync retry, CP-51 C6 live-verification evidence quality.
- severity: medium -- no data loss (there genuinely is nothing to sync for these runs), but a persistently misleading "not fully synced" signal and repeated wasted sync attempts.

## 6. Root Cause

- hypothesis: initially suspected this might be conflated with BUG-309 (the in-memory `SyncStatus` reporting gap) -- i.e. that these runs might actually be succeeding but just mis-reported.
- confirmed cause: distinct from BUG-309. Live re-test after the BUG-309 fix (with a genuinely restarted, fixed runner) still showed these 4 runs with blank `syncStatus`, and a direct `POST .../sync-chat` call returned a real `session_unavailable` error (`"session data not found on this machine"`) -- a genuine, repeatable failure, not a reporting artifact. `BuildChatSessionSyncManifest` (`chat_session_sync.go:317-319`, pre-fix) detected this exact condition (chat-kind run, no transcript found anywhere) but only ever returned the error -- it never wrote anything to the persisted store recording that this run is permanently unsyncable. The frontend's own `syncStatus: "failed"` + `unavailableReason` (set locally in `syncHistoryRun`'s catch block for this exact error code, per BUG-088's convention) never survives the Navigator's periodic `loadRunHistory()` poll, since the backend has nothing to report back for those fields.
- evidence: `TestBuildChatSessionSyncManifestPersistsUnsyncableOnMissingSession` fails on the pre-fix baseline (`SyncStatus = ""`, want `"unsyncable"`), confirmed via `git stash`. Live re-test: the same 4 Gate-sandbox runs showed `syncStatus: "unsyncable"` after the fix, versus blank before.

## 7. Fix Strategy

- `F-1` `BuildChatSessionSyncManifest`'s `session_unavailable` branch now calls `s.updateLocalSessionSyncStatus` to persist `SyncStatus: "unsyncable"` for that run before returning the error.
- `F-2` `isSyncableRun` (`navigatorHistory.ts`) now excludes `syncStatus === "unsyncable"` in addition to the existing `"synced"` exclusion.

## 8. Validation

- `V-1` New test `TestBuildChatSessionSyncManifestPersistsUnsyncableOnMissingSession` (`bug311_test.go`) -- PASS on the fix (including a second call proving the terminal state is stable, not a one-shot side effect); confirmed FAIL on the pre-fix baseline via `git stash`.
- `V-2` New test `"isSyncableRun permanently excludes an unsyncable run (BUG-311)"` (`navigatorHistory.test.ts`) -- PASS; full suite (12 tests) green, no existing assertion touched.
- `V-3` Existing `TestBuildChatSessionSyncManifestChatRunStillRejectsPlaceholderSession` (the pre-existing test already covering the `session_unavailable` return code) -- still PASS, confirming the HTTP-visible error behavior is unchanged; only the new persisted side effect was added.
- `V-4` `go build ./...`, `go vet ./internal/runner/` -- clean. Regression sweep: only the same 2 pre-existing, unrelated failures as BUG-310 (`TestRestoreChatRunFromDriveMissingActiveAccountHome`, `TestNextAccountHomePathGrokUsesGrokHomePrefix`), both confirmed present on baseline.
- `V-5` Live re-verification: after restarting the dev stack with the fix, all 4 previously-perpetually-failing cancelled runs in project "Gate-sandbox" (`run-35063`, `run-34555`, `run-34067`, `run-33581`) now persist `syncStatus: "unsyncable"`.

## 9. Regression Guard

- tests: `apps/local-runner/internal/runner/bug311_test.go` (`TestBuildChatSessionSyncManifestPersistsUnsyncableOnMissingSession`), `apps/desktop-flowpilot/src/components/navigatorHistory.test.ts` (new `isSyncableRun` case).
- alerts: none.
- audit checks: [CA-400](../../change-audit/CA-400-session-unavailable-persists-unsyncable.md).

## 10. Follow-Up Document Updates

- upstream docs that must change: none -- this corrects a retry-loop defect in an existing error path, not a change to business intent or the sync contract itself.
- notes left unchanged on purpose: no new UI affordance was added to visually distinguish `"unsyncable"` rows from ordinary synced/unsynced ones beyond no longer offering a sync action for them (the per-row sync icon is already gated by `isSyncableRun`) -- a dedicated "can't sync: no local data" indicator is a UX polish item, not required by this fix.
