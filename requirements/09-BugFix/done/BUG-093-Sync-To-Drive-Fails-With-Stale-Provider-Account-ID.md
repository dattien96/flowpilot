# Metadata

- Document ID: `BUG-093`
- Title: `Sync-To-Drive Fails With Stale Provider Account ID`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `—`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `—`
- Related Documents: [BUG-092: History Resume Fails When Persisted Provider Account ID Is Stale](./BUG-092-History-Resume-Fails-When-Persisted-Provider-Account-ID-Is-Stale.md), [CA-108: Fix Sync-To-Drive Stale Account Recovery](../../../change-audit/CA-108-fix-sync-to-drive-stale-account-recovery.md)
- Replaces: `—`
- Tags: `google-drive, chat-sync, provider-account, local-runner, severity-medium`

## AI Quick View

### Summary

- BUG-092 fixed `resumeRun` to recover stale provider account IDs by scanning same-provider homes.
- `BuildChatSessionSyncManifest` (the sync-to-Drive path) still keyed exclusively on the stored `ProviderAccountID`.
- A stale ID returned `account_unavailable` before any session-file lookup, blocking Drive sync for chats that had not been opened since the account ID regenerated.
- Opening the chat first self-healed via BUG-092's fix, but syncing before opening remained broken.

### Current Ask

- Mirror the same stale-account fallback from `ensureResumeReady` into `BuildChatSessionSyncManifest`.
- Add test coverage for the "sync before open with stale account ID" scenario.

### Key Decisions

- `V-1` Stale-account recovery in sync mirrors the BUG-092 resume recovery: scan same-provider registered homes for the exact session ID, prefer the active account.
- `V-2` Recovered account ID is persisted via `updateLocalSessionSyncStatus` so future syncs are fast.
- `V-3` Drive sync integrity checks (SHA256, schema version) remain unchanged.

### Constraints

- Do not alter existing sync conflict resolution or BUG-091 prefix-extension logic.
- Recovery must not search across providers.

### Open Questions

- None.

### Source Refs

- Root-cause analysis from BUG-092 post-fix review, 2026-06-19.
- `apps/local-runner/internal/runner/chat_session_sync.go`
- `apps/local-runner/internal/runner/chat_session_sync_test.go`

## 1. Issue Summary

After provider account IDs regenerate (e.g. `provider-accounts.json` was missing on a prior restart), any Drive sync attempted before the chat is opened fails with `account_unavailable`. Opening the chat first self-heals via BUG-092, but the sync path has no equivalent fallback.

## 2. Parent Links

- impacted coding plan: [Task-069](../../08-Task/inprogress/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)
- impacted tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, local runner, provider account ID regenerated (missing `provider-accounts.json` on prior start), Google Drive chat sync enabled
- reproduction steps:
  1. Allow `provider-accounts.json` to be missing on runner start (IDs regenerate).
  2. Without opening any existing chats from History, click the Drive sync button on one.
- frequency: deterministic when the stored `ProviderAccountID` is no longer present in `provider-accounts.json`

## 4. Expected vs Actual

- expected: sync finds the session file under a surviving same-provider account home and succeeds
- actual: sync returns `account_unavailable` immediately without scanning other homes

## 5. Impact

- users affected: users whose account IDs regenerated while local session files remained
- workflows affected: Drive sync initiated before the chat is first reopened post-ID-regeneration
- severity: medium (workaround: open the chat once first; sync then works)

## 6. Root Cause

- hypothesis: same volatile account ID dependency as BUG-092
- confirmed cause: `BuildChatSessionSyncManifest` calls `resolveAccountHome(session.ProviderKey, session.ProviderAccountID)` and returns `account_unavailable` on a stale ID without attempting the same same-provider home scan that `ensureResumeReady` now performs.
- evidence: code review of `chat_session_sync.go:245`; BUG-092 fix only touched `interactive_resume.go`

## 7. Fix Strategy

- `F-1` After `resolveAccountHome` fails in `BuildChatSessionSyncManifest`, call `locateSessionAcrossProviderAccounts` with the stored `ProviderSessionID`.
- `F-2` On recovery, update `session.ProviderAccountID` and `accountHome` in place and continue.
- `F-3` Persist the repaired account ID via `updateLocalSessionSyncStatus` so future syncs avoid the scan.
- `F-4` Add `TestSyncChatRunWithStaleAccountIDBeforeOpening` to verify sync succeeds without opening first, and that the account ID is repaired in the store.

## 8. Validation

- `V-1` `TestSyncChatRunWithStaleAccountIDBeforeOpening` fails before the fix and passes after.
- `V-2` All existing `TestSyncChatRunToDrive*`, `TestRestoreChatRunFromDrive*`, and `TestSyncedChat*` tests continue to pass.

## 9. Regression Guard

- tests: `chat_session_sync_test.go` covers stale-ID sync recovery; all prior sync/restore tests guard against regression
- alerts: `account_unavailable` is still returned if no same-provider home contains the session file
- audit checks: recovered account ID must match a registered, auth-verified account

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: Drive integrity checks (SHA256, schema) are not altered
