# CA-306: Fix Grok Jira MCP headers shape and stale stdio keys

## Summary

Two independent bugs kept Grok's Jira MCP unusable even after Rovo Basic auth
worked for Claude and Codex:

- **Live ACP (`session/new`):** `grokACPExtraMCPServers` sent the Jira
  entry's `headers` as a `{"Authorization": "..."}` MAP. ACP's
  `HttpMcpServer.headers` is actually an ARRAY of `{name, value}` objects
  (`HttpHeader[]`) — the same shape FlowPilot's own runner-hosted MCP server
  already used for its (empty) header list. Grok rejected `session/new` with
  `Invalid params` (-32602) because Jira was the first entry to ever carry a
  real header value, so the map shape had never been exercised against a
  live Grok session.
- **Static `config.toml`:** `grokMcpServer`'s `Command`/`Args` fields lacked
  `omitempty`, so the remote-HTTP Jira entry serialized `command = ""` /
  `args = []` alongside its `url`/`headers`. Grok reads a present `command`
  key as "this is a stdio server, launch it" — an empty command made the
  entry show as `[unavailable]` in Grok CLI even though the MCP url/auth were
  correct. `ensureGrokJiraMcpConfig` also now force-rewrites (heals) an
  existing config entry that still carries those stale stdio keys, since the
  typed before/after comparison otherwise treats an empty command as
  "unchanged" and never re-writes it.

Mirrors `codexMcpServer`, whose `Command`/`Args` were already `omitempty` for
exactly this reason.

## Why this failed

Reported live: after Claude/Codex successfully read a real Jira ticket via
MCP, the same prompt against Grok in the FlowPilot chat returned
`Invalid params` on `session/new`. Separately, `grok mcp` in the standalone
Grok CLI showed `jira [unavailable]` even after "Configure Providers" wrote
the entry — inspecting the user's live `~/.grok/config.toml` showed the
remote HTTP entry carrying `command = ""` and `args = []`.

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'TestGrokACPExtraMCPServers|TestEnsureJiraMcpProviderConfigDispatchesToGrok|TestEnsureGrokJiraMcpConfigHealsStaleStdioKeys|TestPreflightJiraMcp|TestEnsureGrokGoogleDriveMcpConfig|TestFlowpilotClaudeExtraMCPServers' -count=1
```

## Source

- CP-05-06, live user report (Grok "Invalid params" in chat; Grok CLI
  `[unavailable]`).

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: bugfix
summary: fix Grok ACP Jira headers array shape and config.toml stale stdio keys causing Invalid params / [unavailable]
# --->8---
