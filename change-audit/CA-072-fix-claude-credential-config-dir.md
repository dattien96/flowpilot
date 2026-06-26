# CA-072: Fix Claude Credential Config Dir

## Scope

- Corrected a Claude local-runner regression where the account could be selected but the live Claude CLI returned `Not logged in - Please run /login`.
- Touched Claude auth detection and live Claude adapter environment construction.
- Kept the previous Codex/BOM fix behavior intact while tightening what counts as a connected Claude account.

## Completed

- Added `.claude/.credentials.json` to Claude auth discovery for default and stored account homes.
- Changed Claude auth validation to require credential-bearing token fields such as `claudeAiOauth.accessToken` or `claudeAiOauth.refreshToken` instead of accepting `.claude.json` email metadata.
- Updated the live Claude adapter registry path to set `HOME`, `XDG_CONFIG_HOME`, Windows profile variables when applicable, and `CLAUDE_CONFIG_DIR=<account home>/.claude`.
- Updated focused regression tests to use credential files and added an assertion for the adapter's `CLAUDE_CONFIG_DIR`.
- Added a false-positive guard proving `.claude.json` email metadata alone does not keep a Claude account connected.

## Verification

- `go test ./internal/runner -run "Test(ClaudeRegistryIgnoresStaleUsageLimitMetadata|ClaudeRegistryUsesCredentialConfigDirForAccount|ListProviderAccountsKeepsStoredDefaultClaudeAccountConnected|ListProviderAccountsKeepsStoredDefaultClaudeAccountConnectedWithBOMAuth|ListProviderAccountsMarksClaudeMetadataOnlyAccountFailed|MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata)$"` - PASS

## Residual Notes

- Full runner-package tests were not rerun after this follow-up; earlier in the same session they failed on unrelated Google Drive MCP setup and Windows shell environment assumptions.
- GitNexus tools were not exposed in this session, so required symbol impact checks were documented as unavailable and replaced with local code inspection.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-072
change_type: fix
summary: Fix Claude Credential Config Dir
# --->8---
