# CA-305: Fix Claude account detection for OS-keychain credential storage

## Summary

`claudeAuthFileLooksValid` required a non-empty `accessToken`/`refreshToken`
string inline in `~/.claude/.credentials.json`. On Windows (Credential
Manager) and macOS (Keychain), Claude Code stores the real OAuth token in the
OS keychain and writes that file with EMPTY `accessToken`/`refreshToken`
strings, keeping only session metadata (`expiresAt`, `subscriptionType`,
`scopes`, `refreshTokenExpiresAt`).

That made every Windows/macOS Claude installation invisible to provider-
account discovery: `syncProviderAccounts` never marked the account
"connected", so "Configure Providers" skipped Claude entirely and
Jira/Firebase/Telegram MCP config never landed in `~/.claude.json` even
though Claude Code was fully logged in.

A `claudeAiOauth` block carrying real session metadata (`subscriptionType` or
a positive `expiresAt`/`refreshTokenExpiresAt`) now counts as authenticated
even when the token strings are empty. This only relaxes the `claudeAiOauth`
case — a bare `oauthAccount`-only `.claude.json` (metadata with no real
session) still reports "failed".

## Why this failed

Reported live: after connecting Jira and running "Configure Providers", the
account list showed Codex/Gemini/Grok accounts but no Claude account, even
though Claude Code was logged in on the same Windows machine. Reading the
user's `~/.claude/.credentials.json` directly showed a `claudeAiOauth` block
with empty `accessToken`/`refreshToken` and populated `expiresAt`/
`subscriptionType` — the keychain-backed storage shape the validator did not
handle.

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'TestListProviderAccountsConnectsKeychainClaudeAccount|TestListProviderAccountsMarksClaudeMetadataOnlyAccountFailed|TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnected' -count=1
```

## Source

- CP-05-06 (Configure Providers dependency), live user report (Claude
  missing from provider-account list on Windows).

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: bugfix
summary: detect Claude accounts that store OAuth tokens in the OS keychain instead of inline in .credentials.json
# --->8---
