# CA-301: Jira Rovo MCP via personal API token (Basic)

## Summary

Primary Jira remote-MCP auth is now the official Atlassian headless path:

- `Authorization: Basic base64(email:apiToken)` from connected keyring credentials
- URL `https://mcp.atlassian.com/v1/mcp`
- Optional Bearer override (service account / paste) → `…/mcp/authv2`
- Configure Providers no longer requires bearer paste
- Live merge (`jiraLiveMCPServer`) uses the same resolver
- Codex Basic uses `http_headers` (not `bearer_token_env_var`) so Basic is not wrapped as Bearer
- Admin/desktop UI: clickable links to enable Rovo MCP API token auth + create MCP-scoped token
- MCP Test Console UI retired (route shows retired notice); REST `executeJiraMcpPrompt` kept in runner
- Desktop `secretConfigFields.jira = ["apiToken"]` (Supabase must never store token)
- `EnsureClaudeJiraMcpConfig` picks URL from Authorization (`Basic` → `/v1/mcp`, `Bearer` → `/authv2`)
- Basic no-override tests for Claude/Codex/Gemini/Grok + Grok ACP

## Security decision (post review)

- **Supabase:** no `apiToken` in `config_encrypted` (desktop strip + admin strip).
- **Keyring:** durable secret store.
- **Local AI provider config files:** may hold `Authorization: Basic base64(email:apiToken)` after Configure Providers — same class as prior Bearer-in-config; accepted for v1 so provider CLIs can load remote MCP without a custom secret broker. Do not commit account homes; do not log config file bodies.

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'Jira|GrokACP|GrokServer|ExtraMCP|PreflightJira|EnsureGrok|EnsureClaudeJira' -count=1
```

## Source

- CP-05-06 DOD-2 partial, implementation_plan.md Jira-MCP-API-Token-Auth
- https://support.atlassian.com/atlassian-rovo-mcp-server/docs/configuring-authentication-via-api-token/

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: feature
summary: Wire Rovo MCP with Basic API token; retire Test Console UI; admin Rovo enable links
# --->8---
