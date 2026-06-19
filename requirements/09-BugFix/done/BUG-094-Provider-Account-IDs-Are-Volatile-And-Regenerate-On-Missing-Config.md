# Metadata

- Document ID: `BUG-094`
- Title: `Provider Account IDs Are Volatile And Regenerate On Missing Config`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `—`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Child Documents: `—`
- Related Documents: [BUG-092: History Resume Fails When Persisted Provider Account ID Is Stale](./BUG-092-History-Resume-Fails-When-Persisted-Provider-Account-ID-Is-Stale.md), [BUG-093: Sync-To-Drive Fails With Stale Provider Account ID](./BUG-093-Sync-To-Drive-Fails-With-Stale-Provider-Account-ID.md), [CA-109: Fix Provider Account ID Volatility](../../../change-audit/CA-109-fix-provider-account-id-volatility.md)
- Replaces: `—`
- Tags: `provider-account, local-runner, severity-medium, stability`

## AI Quick View

### Summary

- `newProviderAccountID()` generates a random 16-byte hex ID every call.
- `syncProviderAccounts` auto-creates default (slot-0) and managed (slot-N) account entries with these random IDs whenever they are absent.
- If `provider-accounts.json` is missing or resolves to a different path, all auto-created IDs are regenerated as new random values.
- Every feature that persists and later re-resolves an account ID (resume, sync, cross-account copy) becomes broken until the BUG-092/093 recovery fallbacks self-heal.

### Current Ask

- Use deterministic IDs for auto-created provider accounts so that ID regeneration produces the same stable value for a given provider+home combination.

### Key Decisions

- `V-1` Deterministic IDs are derived from SHA-256 of `providerKey + ":" + normalizedHomePath`, truncated to 32 hex chars — same length as random IDs.
- `V-2` Existing entries in `provider-accounts.json` keep their stored IDs unchanged (backward-compatible).
- `V-3` `newProviderAccountID()` remains for manually-created accounts (e.g. `ConnectProviderAccount`); only auto-sync entries use deterministic IDs.

### Constraints

- Must not change the format or length of account IDs (32 hex chars).
- Must not require migration of existing `provider-accounts.json` files.
- Must not break any existing test that manually specifies an account ID.

### Open Questions

- None.

### Source Refs

- Root-cause analysis from BUG-092 post-fix review, 2026-06-19.
- `apps/local-runner/internal/runner/provider_accounts.go`

## 1. Issue Summary

Provider account IDs stored in session records become stale whenever `provider-accounts.json` is missing or at a different path on runner start, because `syncProviderAccounts` mints brand-new random IDs for the same physical account homes. The root bug (BUG-092) is mitigated by fallback recovery, but the root cause — random IDs for structurally stable accounts — was not addressed.

## 2. Parent Links

- impacted coding plan: [Task-069](../../08-Task/inprogress/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)
- impacted tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- impacted system spec: `—`

## 3. Environment and Reproduction

- environment: local runner, `provider-accounts.json` missing or at a new path on restart
- reproduction steps:
  1. Start runner; create chats; stop runner.
  2. Delete `provider-accounts.json` (or change the path resolution env vars).
  3. Restart runner.
  4. Open or sync any existing chat — stored account IDs are now orphaned.
- frequency: deterministic when the config file is missing at startup

## 4. Expected vs Actual

- expected: auto-created account entries for the same provider+home always produce the same ID, so stored session records remain valid across restarts
- actual: each regeneration of `provider-accounts.json` produces new random IDs, orphaning all stored session account references

## 5. Impact

- users affected: any user whose `provider-accounts.json` is missing or path-shifted at runner start
- workflows affected: history resume, Drive sync, cross-account copy — any workflow that re-resolves a stored account ID
- severity: medium (BUG-092/093 recovery fallbacks mask the symptom; this fix prevents the cause)

## 6. Root Cause

- hypothesis: random ID generation for structurally stable default accounts
- confirmed cause: `newProviderAccountID()` is called for every new entry in `syncProviderAccounts`, including default (slot-0) and managed (slot-N) accounts. The same physical home path gets a different ID each time the entry is recreated.
- evidence: `provider_accounts.go:syncProviderAccounts` and `syncManagedProviderAccounts` both call `newProviderAccountID()`; `loadProviderAccountState` returns empty state on missing file

## 7. Fix Strategy

- `F-1` Add `deterministicProviderAccountID(providerKey, homePath string) string` using `SHA-256(providerKey + ":" + normalized(homePath))[:32 hex chars]`.
- `F-2` Replace `newProviderAccountID()` with `deterministicProviderAccountID` in `syncProviderAccounts` (slot-0 default creation) and `syncManagedProviderAccounts` (slot-N managed creation).
- `F-3` Leave `newProviderAccountID()` in `ConnectProviderAccount` (user-initiated, not auto-discovered).

## 8. Validation

- `V-1` Same provider+home combination always produces the same ID across multiple `syncProviderAccounts` calls.
- `V-2` All existing `TestListProviderAccounts*` and `TestSyncedChat*` tests continue to pass.
- `V-3` Existing entries in a `provider-accounts.json` are not touched (ID field preserved).

## 9. Regression Guard

- tests: existing provider-accounts tests guard against regressions; deterministic ID property is verified by repeating `syncProviderAccounts` in tests
- alerts: none required — deterministic IDs are silently identical across regenerations
- audit checks: `deterministicProviderAccountID` must be collision-free for distinct provider+home combinations

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-14 note that account IDs for auto-discovered homes are now stable
- notes left unchanged on purpose: user-initiated `ConnectProviderAccount` still uses random IDs (user creates the slot; the home path is unknown before creation)
