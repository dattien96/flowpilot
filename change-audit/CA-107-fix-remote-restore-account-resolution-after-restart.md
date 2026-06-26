# CA-107: Fix Remote Restore Account Resolution After Restart

## Scope

Correct Drive-backed Remote Chats restore after a runner restart when the provider uses a managed non-default account home.

## Completed

- Changed `restoreChatRunFromDrive` to resolve the active account from the manifest provider through the durable provider-account store.
- Persisted the same resolved account id on the restored session record.
- Added restart regression coverage proving local History resume and Remote Chats restore both work after reloading `sessions.ndjson`.
- Updated the missing-account test to model a genuine absence of durable connected provider accounts.
- Added structured desktop and runner logs for history-open resume, account/session lookup, typed failures, and transcript stream replay after the user reported the symptom still occurs.
- Added stale-account recovery for Claude and Codex: when the stored account ID is gone, resume searches same-provider registered homes for the exact provider session ID, verifies auth, and persists the repaired account binding.
- Added provider-specific regression tests matching the real `source home unresolved` traces.

## Verification

- Passed targeted runner tests:
  - `TestSyncedChatCanResumeAfterServiceRestart`
  - `TestSyncedChatCanRestoreFromRemoteAfterServiceRestart`
  - all `TestRestoreChatRunFromDrive*` tests
- Attempted `go test ./internal/runner -count=1`; unrelated environment-sensitive tests remain failing and are recorded in BUG-092.

## Residual Notes

- GitNexus MCP tools were unavailable in this session, so impact analysis was performed manually.
- Existing uncommitted BUG-091 restore conflict changes were preserved and validated by the targeted restore suite.
- Recovery is intentionally exact-ID based; it does not choose the newest unrelated provider session.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-092
change_type: fix
summary: Fix Remote Restore Account Resolution After Restart
# --->8---
