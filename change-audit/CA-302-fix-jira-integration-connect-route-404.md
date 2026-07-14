# CA-302: Fix Jira integration connect route 404

## Summary

Fixed the desktop MCP integration connect path after the global-scope Jira/Firebase/Telegram work exposed a route mismatch:

- Desktop `RunnerAdminRepository.testIntegration(...)` now posts to the runner's canonical endpoint: `/integrations/{integrationId}/connection`.
- Removed the redundant `integrationId` field from that request body because the runner already receives it from the path.
- Added a backward-compatible runner alias `POST /integrations/connect` that still accepts the old body shape and forwards it to `TriggerIntegrationConnection`.
- Desktop Jira creation now treats a same-site same-scope duplicate as an update/reconnect of the existing record instead of hard-failing, which recovers stale `pending` rows left behind by the old 404 path.
- Desktop MCP settings now expose `Delete` on existing integrations, removing the runner-side connection/keyring record first and then deleting the Supabase integration row.
- Updated desktop MCP comments/helpers so the secret-handling docs reference the correct endpoint.

## Why this failed

The runner has long exposed `POST /integrations/{id}/connection`, but the desktop client was posting to `/integrations/connect`. Creating a Jira instance inserts the integration row first, then immediately calls the connect endpoint, so the bad path surfaced as a `404 Not Found` during instance creation.

That left a second-order problem: the half-created Jira row already existed in Supabase, but its API token had correctly never been persisted there. A retry then hit the desktop duplicate guard and could not use the list-level `Test` button to recover, because the saved record had no secret to reconnect with. The create flow now reuses that row and reconnects it with the freshly-entered secret.

## Verification

```bash
./apps/desktop-flowpilot/node_modules/.bin/tsc -p apps/desktop-flowpilot/tsconfig.json --noEmit
go test ./internal/runner -run 'TestTriggerIntegrationConnection|TestRunMcpTestDoesNotBackfillLegacyJiraProjectScope' -count=1
go test ./internal/cli -count=1
```

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: bugfix
summary: fix desktop Jira integration connect route mismatch and add legacy runner alias
# --->8---
