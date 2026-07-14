# CA-308: Sync Codex Jira MCP to the live account each turn (stale-account fix)

## Summary

Codex answered Jira MCP prompts as a **different Atlassian account** than
Claude and Grok on the same workspace. Root cause is an architectural
asymmetry, not a bad token:

- Claude and Grok get a per-turn `extraMCPServers` live merge
  (`flowpilotClaudeExtraMCPServers` → `jiraLiveMCPServer` →
  `resolveJiraMcpAuth("")`), so their Jira MCP always uses the
  **currently-connected** workspace credential regardless of what their static
  config files hold.
- Codex has **no** per-turn injection channel. Its shared app-server
  (`ensureCodexAppServer`) reads `mcp_servers` from `config.toml` once at
  process start. So the token an earlier "Configure Providers" run wrote (for
  whatever account was connected *then*) silently persisted, and Codex kept
  answering as that stale account after the workspace connected a new one.
  Live evidence: both Codex and Grok `config.toml` carried the same old
  `Basic base64(trashname899@gmail.com:…)` entry, yet Grok answered as the new
  account (live merge) while Codex answered as the old one (static file).

The fix has three layers so both FlowPilot's Codex **and** standalone
`codex-cli` (which reads `config.toml` directly) always see the connected
account:

### 1. On-disk config is rewritten when the Jira connection changes (codex-cli)

- `rePushJiraConfigToConnectedProviders()` (jira_mcp_provider_config.go), called
  from `TriggerIntegrationConnection`'s Jira connect-success path
  (runner.go): re-writes each **locally-authenticated** provider's static Jira
  MCP config to the newly-connected account. This is what makes `codex-cli`
  pick up an account switch without a manual "Configure Providers" run. Targets
  only providers with a detected auth file (`DetectDefaultAccountHomePath` is
  pure detection — never creates a home), and is best-effort (a per-provider
  failure is logged and skipped, never blocks the connect).

### 2. Configure-Providers writer no longer wipes other config keys

- `ensureCodexJiraMcpConfig` rewritten from the typed whole-document
  `codexConfig` round-trip to a **generic-map splice** (`spliceCodexJiraEntry` +
  `writeCodexConfigDoc`). `codexConfig` models only `mcp_servers`, so marshaling
  it back dropped every other top-level key (`model`, `[projects]` trust levels,
  `[tui]`, `[windows]`) — a latent data-loss bug. It now touches only
  `mcp_servers.jira` and errors (rather than silently resetting) on an
  unparseable config.

### 3. Per-turn live sync inside FlowPilot (belt-and-suspenders)

- `syncCodexJiraMcpLive(accountHomePath)`: on every Codex turn, rewrite
  `config.toml [mcp_servers.jira]` to the currently-connected credential (same
  `spliceCodexJiraEntry` splice) and return `changed`. It **only ever updates**
  (never removes): `resolveJiraMcpAuth` fails both on a genuine disconnect and
  on a transient keyring read, and stripping a working entry on a transient
  error is worse than leaving a possibly-stale one, so a resolve failure is a
  no-op.
- `resetCodexAppServer()` (codex_appserver_process.go): tears down the shared
  app-server so the next `ensureCodexAppServer` spawns fresh. Codex loads
  `mcp_servers` only at boot, so after the token is rewritten the running
  process must be respawned to serve the new account.
- `provider_registry.go` Codex `newAdapter`: calls `syncCodexJiraMcpLive`
  before `ensureCodexAppServer`, and `resetCodexAppServer()` when it changed.
  Steady state (same account) → `changed=false` → no rewrite, no respawn.

Shared helpers `codexJiraExpectedServer` / `codexJiraServerMatches` /
`spliceCodexJiraEntry` are used by all three layers so they can never build or
compare a different entry.

> NOTE: all three layers are runner-side Go — the running local-runner must be
> rebuilt/restarted for them to take effect. Layer 1 is what corrects the file
> for codex-cli; layers 2–3 keep FlowPilot's own Codex correct and non-destructive.

## Why

Reported live: "dùng jira mcp đọc cho tôi 1 bug bất kì" returned account A in
Claude and Grok chat but account B in Codex chat. Diagnosed to the missing
live-injection channel for Codex.

## Verification

```bash
cd apps/local-runner
go build ./...
go test ./internal/runner -run 'Codex|Jira' -count=1   # 186 passed, 9 failed
```

New tests: `TestSyncCodexJiraMcpLiveRewritesStaleTokenPreservingOtherKeys`
(rewrites stale token to the connected account, preserves `model`/`[projects]`/
`[tui]`, idempotent), `TestSyncCodexJiraMcpLiveNoopWhenNotConnected` (no write,
no error when Jira unresolved), and
`TestEnsureCodexJiraMcpConfigPreservesOtherTopLevelKeys` (the Configure-Providers
path now preserves `model`/`model_reasoning_effort`/`[projects]`/`[tui]`/
`[windows]` instead of dropping them). The 9 failures (`TestCodexResume*` /
cross-account / skills-merge) reproduce identically on a clean tree
(`git stash`) — missing codex binary / Windows env, unrelated.

## Source

- CP-05-06, live user report (Codex answered as a different Atlassian account).

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: bugfix
summary: sync Codex config.toml jira MCP entry to the currently-connected account each turn (respawn app-server on change) so Codex no longer answers as a stale account
# --->8---
