# CA-304: Fix Jira credential verify to probe the Rovo MCP endpoint

## Summary

`verifyJiraCredential` gated Jira connect/test on a single
`GET /rest/api/3/project/{key}` against the classic site REST API instead of
the Atlassian Rovo MCP endpoint the credential is actually used with:

- A modern *scoped* API token — the type Rovo's Teamwork Graph tools
  require — frequently returns 401 on classic REST endpoints, so the old
  gate rejected exactly the token type the feature needs.
- A 404 on `/project/{key}` only meant "this account can't browse that
  project", not "bad credential", yet it hard-blocked saving the credential.

`verifyJiraCredential` now POSTs a minimal MCP `initialize` handshake
directly to `mcp.atlassian.com/v1/mcp` with the same `Basic email:apiToken`
header a connected integration sends on every live turn. Only HTTP 401/403
means the credential was rejected; any other status means auth passed. A
transport error no longer blocks the save — it surfaces as a warning.

Also added `normalizeJiraWorkspaceURL`: a pasted Jira board deep link (e.g.
`.../jira/software/projects/SCRUM/boards/1`) is reduced to its origin before
being stored.

## Why this failed

Reported live: a user's modern Atlassian API token (scoped, created for
Rovo/MCP use) returned `401 Unauthorized` from the old classic-REST verify
gate, blocking the connect flow entirely, even though the same token
authenticates fine against the actual Rovo MCP endpoint the integration uses
at runtime.

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'TestVerifyJiraCredential|TestNormalizeJiraWorkspaceURL|TestTriggerIntegrationConnectionUsesJiraApiToken|TestVerifyMcpBackendUsesRequestedJiraIntegrationScope' -count=1
```

## Source

- CP-05-06 DOD-2, live user report (401 on connect with a scoped token).

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: bugfix
summary: verify Jira credential against the Rovo MCP endpoint instead of classic REST, unblocking scoped API tokens
# --->8---
