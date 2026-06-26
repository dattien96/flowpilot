# CA-070: Fix Claude Stale Usage Metadata Block

## Scope

- Corrected Claude local-runner behavior after quota reset when local Claude metadata still contains `cachedExtraUsageDisabledReason: "out_of_credits"`.
- Corrected provider account sync so a stored default Claude account is not marked failed solely because the runner process home differs from the account's saved home path.
- Touched provider account sync, the live Claude provider adapter factory, and the legacy `claude_stream_json` session execution path.
- Added account-sync, registry, and session regression coverage for reset accounts that still have stale local usage metadata.

## Completed

- Removed the local metadata usage-limit preflight from Claude adapter creation so the active connected account is passed to Claude instead of being rejected before execution.
- Removed the same stale-cache preflight from the `claude_stream_json` session send path.
- Updated provider account sync to check the stored default account `home_path` for valid auth before marking it failed.
- Kept runtime usage-limit normalization for actual Claude stderr and result-frame failures so real quota errors still show the clean FlowPilot message.
- Backed up and repaired the local FlowPilot provider account file so `C:\Users\dat.nguyen` is again the active connected Claude account.

## Verification

- `go test ./internal/runner -run "Test(ListProviderAccountsKeepsStoredDefaultClaudeAccountConnected|ClaudeRegistryIgnoresStaleUsageLimitMetadata|SendMessageClaudeStaleUsageLimitMetadataStillRunsCommand|SendMessageClaudeResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata|SendTurnWithRetryDoesNotRetryUsageLimit)$"` - PASS
- `go test ./internal/runner` - FAIL from existing Google Drive/Codex environment-dependent tests, not from the Claude-focused changes.

## Residual Notes

- This does not change Claude account selection; `ResolveProviderAccount` still prefers the active connected account.
- If Claude itself returns a current quota error, FlowPilot will still show `Claude usage limit reached...`.
- Local config backup: `C:\Users\dat.nguyen\AppData\Roaming\FlowPilot\provider-accounts.json.bak-20260615-121123`.
- GitNexus tools were not exposed in this session, so the required symbol impact check was documented as unavailable and replaced with local code inspection.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-070
change_type: fix
summary: Fix Claude Stale Usage Metadata Block
# --->8---
