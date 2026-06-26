# CA-109: Fix Provider Account ID Volatility

## Scope

Replace random ID generation for auto-discovered default and managed provider accounts with deterministic IDs derived from the provider key and home path.

## Completed

- Added `deterministicProviderAccountID(providerKey, homePath string) string` using SHA-256 of `providerKey + ":" + normalized(homePath)`, truncated to 32 hex chars.
- Replaced `newProviderAccountID()` with `deterministicProviderAccountID` in `syncProviderAccounts` (slot-0 default creation) and `syncManagedProviderAccounts` (slot-N managed creation).
- `ConnectProviderAccount` (user-initiated) continues to use `newProviderAccountID()` since the home path is not known until after creation.

## Verification

- Existing entries in `provider-accounts.json` are not modified (ID field is only set on new entry creation).
- All existing `TestListProviderAccounts*` and `TestSyncedChat*` tests pass.
- Deterministic property: two consecutive `syncProviderAccounts` calls on the same empty state produce identical IDs.

## Residual Notes

- Existing users with random IDs in their `provider-accounts.json` are unaffected; their existing IDs are preserved on load.
- New installations and any post-regeneration state will produce stable, repeatable IDs.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-109
change_type: fix
summary: Fix Provider Account ID Volatility
# --->8---
