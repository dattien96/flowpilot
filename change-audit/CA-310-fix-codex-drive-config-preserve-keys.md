# CA-310: Codex follows the selected Google Drive account (parity with Claude/Grok)

## Summary

Codex kept authenticating to Google Drive as a stale account after the user
switched the active Drive account in Settings: Claude updated immediately, Codex
did not. Root cause is the same asymmetry as the Jira bug (CA-308): Claude and
Grok **re-resolve google-drive live every turn** in
`flowpilotClaudeExtraMCPServers` (reading the currently-selected account + its
keyring token), while Codex reads only its **static `config.toml`**, whose
refresh token was frozen at the last Configure Providers run. The credential
store is NOT corrupt — Claude proves the current account resolves correctly.

Three parts:

1. **Follow the account on switch (the fix that reaches codex-cli).**
   `rePushGoogleDriveConfigToStaticProviders`, called from
   `SaveGoogleDriveWorkspaceConfig` whenever the active MCP account changes,
   refreshes the google-drive credential env in every Codex account home's
   `config.toml` to the newly-selected account. Independent of any Codex turn or
   adapter, so it fixes standalone `codex-cli` too. Best-effort.

2. **Per-turn live sync (belt-and-suspenders for the app-server path).**
   `syncCodexGoogleDriveMcpLive` re-resolves and refreshes the credential env on
   each Codex turn; wired into the Codex app-server adapter alongside
   `syncCodexJiraMcpLive`, respawning the app-server when either changed. (Note:
   the app-server path is gated behind `FLOWPILOT_CODEX_APPSERVER` and off by
   default, which is why part 1 — not this — is what fixes the common case.)
   Both refresh ONLY the account/token env keys, preserving command/args/mode/
   approval and every other config key.

3. **Writer no longer wipes other keys.** `ensureCodexGoogleDriveMcpConfig`
   moved off the `mcp_servers`-only `codexConfig` typed round-trip (which dropped
   `model`/`[projects]`/`[tui]`/`[windows]` on rewrite — same latent bug as the
   Jira writer) to the shared **generic-map splice** (`spliceCodexServerEntry` +
   `writeCodexConfigDoc`). Unparseable config is recovered (reset+rewrite), not
   errored, so `TestEnsureGoogleDriveMcpProviderConfig_RewritesInvalidExisting
   Configs` still passes; a parseable config keeps all its keys.

`spliceCodexJiraEntry` was generalized into `spliceCodexServerEntry(doc,
serverName, expected, matches)` so the Jira writer, the Drive writer, and both
live syncs share one compare/build/splice path.

## Why

Reported live: switching the active Google Drive account updated Claude but left
Codex on the old account (`hothuy251271@gmail.com` instead of the selected one).
Diagnosed to Codex lacking the per-turn live merge Claude/Grok have — a
config-propagation gap, not a credential corruption.

## Verification

```bash
cd apps/local-runner
go build ./...
go test ./internal/runner -run 'GoogleDrive|Jira|Codex' -count=1   # 293 passed, 11 failed
```

New test `TestSyncCodexGoogleDriveMcpLiveRestoresStaleTokenPreservingKeys`:
seeds the selected account's token, freezes a stale token + an unrelated
top-level key into `config.toml`, and asserts the sync restores the current
token, drops the stale one, preserves `model`, and is idempotent.

The 11 failures reproduce on a clean tree (`git stash`): 9 `TestCodexResume*` /
cross-account / skills-merge (missing codex binary / Windows env) and 2
`TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted` /
`TestParseGoogleDriveProxyMcpInvocation_AcceptsGoRunFallback` (Windows path
env) — all unrelated to this change.

## Source

- Live user report (Codex Google Drive account mismatch), CP-05-03.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-03
change_type: bugfix
summary: rewrite Codex Google Drive config writer to a generic-map splice so it preserves model/[projects]/[tui]/[windows] instead of dropping them on rewrite
# --->8---
