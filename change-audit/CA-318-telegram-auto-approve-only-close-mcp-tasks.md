# CA-318: Telegram auto-approve only + close MCP tasks 226–233

## Summary

Product decision: drop the per-send Telegram approval queue (G3) and the built-in-flow E2E expectation; keep the integration **auto-approve** toggle as the sole send gate (user-verified). Removed `telegram_proxy_approval.go`, HTTP routes, client-core APIs, and the Pending Approvals UI panel. Moved Task-226..233 to `done/` with OAuth browser path removed from scope (API token auth is v1). Updated Task-233, CP-05-05, Task-228, Task-234, BUG-281, CA-314 docs to match.

## Code

- Deleted `telegram_proxy_approval.go` + tests; simplified `telegram_proxy_mcp.go` / `telegram_loopback.go` to `autoApprove`-only.
- Removed `/telegram-proxy-approvals` routes from `cli/root.go`.
- `McpSettings.tsx`: removed queue panel; clarified **Allow Telegram sends** / **Enable auto-approve** copy.
- `flowpilot-client-core`: removed `TelegramApprovalRecord` and repository methods.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-233
change_type: refactor
summary: Remove Telegram per-send approval queue; keep auto-approve toggle as sole send gate; close MCP task docs 226-233
# --->8---