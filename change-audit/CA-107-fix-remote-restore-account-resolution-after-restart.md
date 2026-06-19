# CA-107: Fix Remote Restore Account Resolution After Restart

## Scope

Correct Drive-backed Remote Chats restore after a runner restart when the provider uses a managed non-default account home.

## Completed

- Changed `restoreChatRunFromDrive` to resolve the active account from the manifest provider through the durable provider-account store.
- Persisted the same resolved account id on the restored session record.
- Added restart regression coverage proving local History resume and Remote Chats restore both work after reloading `sessions.ndjson`.
- Updated the missing-account test to model a genuine absence of durable connected provider accounts.

## Verification

- Passed targeted runner tests:
  - `TestSyncedChatCanResumeAfterServiceRestart`
  - `TestSyncedChatCanRestoreFromRemoteAfterServiceRestart`
  - all `TestRestoreChatRunFromDrive*` tests
- Attempted `go test ./internal/runner -count=1`; unrelated environment-sensitive tests remain failing and are recorded in BUG-092.

## Residual Notes

- GitNexus MCP tools were unavailable in this session, so impact analysis was performed manually.
- Existing uncommitted BUG-091 restore conflict changes were preserved and validated by the targeted restore suite.
