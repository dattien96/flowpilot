# CA-108: Fix Sync-To-Drive Stale Account Recovery

## Scope

Mirror the BUG-092 stale-account recovery fallback into `BuildChatSessionSyncManifest` so Drive sync succeeds even when the persisted `ProviderAccountID` no longer exists.

## Completed

- Added stale-account fallback in `BuildChatSessionSyncManifest`: when `resolveAccountHome` fails, calls `locateSessionAcrossProviderAccounts` with the stored `ProviderSessionID`, recovers the account and home, repairs the persisted account ID via `updateLocalSessionSyncStatus`.
- Added `TestSyncChatRunWithStaleAccountIDBeforeOpening` to verify sync succeeds without opening the chat first, and that the account ID is repaired in the store.

## Verification

- `TestSyncChatRunWithStaleAccountIDBeforeOpening` passes.
- All existing `TestSyncChatRunToDrive*`, `TestRestoreChatRunFromDrive*`, and `TestSyncedChat*` tests pass.

## Residual Notes

- Recovery is exact-session-ID based; it never selects an unrelated session file.
- `account_unavailable` is still returned when no same-provider home contains the exact session file.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-092
change_type: fix
summary: Fix Sync-To-Drive Stale Account Recovery
# --->8---
