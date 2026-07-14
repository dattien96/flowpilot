# CA-307: Add error modal for MCP connection failures, remove Jira OAuth override field

## Summary

- `testIntegration` returns `{ message, ok }` instead of a bare string,
  reading the runner's `requestStatus`/`integrationStatus` so the caller can
  tell a rejection from a success message.
- `McpSettings` surfaces every catch block and every rejected connection
  outcome (load, create/test, backend action, delete, Telegram approval
  decision, Configure Providers) through a blocking error modal instead of
  inline `settings-feedback` text, reusing the existing
  `settings-modal-backdrop` pattern from the project-delete modal.
- Fixes a latent bug: a failed test previously still marked the integration
  "connected" and reset the create/edit form; it now marks it "failed" and
  keeps the form open so the token can be corrected and retried.
- Removed the Jira "Authorization override (optional)" field from Configure
  Providers: that field sent an OAuth/service-account Bearer token, a
  different auth mechanism than the email + Atlassian API token (Basic) path
  this integration otherwise always uses, and was a common source of
  confusion (pasting the wrong credential type into the wrong field).

## Why

Reported live: MCP connection errors (a rejected Jira verify, a runner
connection failure) rendered as the same inline feedback line as ordinary
success messages, so a failed connect/test was easy to miss — the user asked
for errors to "show by modal instead".

## Verification

```bash
./apps/desktop-flowpilot/node_modules/.bin/tsc -p apps/desktop-flowpilot/tsconfig.json --noEmit
cd apps/desktop-flowpilot && npx vite build
```

## Source

- Live user request ("fix show lỗi bằng modal đi").

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-06
change_type: feature
summary: surface MCP integration errors in a blocking modal instead of inline feedback text; remove Jira OAuth override field
# --->8---
