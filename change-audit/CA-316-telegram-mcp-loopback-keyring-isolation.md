# CA-316: Telegram MCP loopback for provider HOME keyring isolation

## Problem

Grok multi-account isolation sets `HOME=/Users/tiendat/.grokHome2` on the provider process. The child `flowpilot_telegram` MCP inherited that HOME and called `resolveConnectedTelegramCredential` / macOS keyring directly, which failed with `secret not found in keyring`. Codex worked because it uses the normal user home. BUG-281 option 3: keep provider HOME isolation; move keyring + Bot API send into the main runner.

## Fix

1. **Runner endpoint** `POST /internal/mcp/telegram/send` (`TelegramLoopbackSendPath`)
   - Loopback-only (`isLoopbackRequest`)
   - Auth via short-lived/workspace token (`FLOWPILOT_RUNNER_MCP_TOKEN`, persisted under `.flowpilot/telegram-mcp-loopback-token`, mode 0600)
   - Resolves bot token from runner keyring, honors `autoApprove` gate + `sendMessage`

2. **`telegram-mcp` child**
   - When `FLOWPILOT_RUNNER_URL` + `FLOWPILOT_RUNNER_MCP_TOKEN` are present: thin loopback client (no keyring, no bot token)
   - Incomplete loopback env → `MCP_UNAVAILABLE` (no direct keyring fallback in provider mode)
   - Without loopback env: direct keyring mode remains for manual `flowpilot telegram-mcp`

3. **Provider config / live merge**
   - `expected*TelegramMcpServer` and `telegramLiveMCPServer` inject loopback URL + token env
   - Bot token never written to provider TOML/JSON or MCP env

## Tests

- `TestTelegramMcpLoopbackRequiresRunnerToken`
- `TestTelegramMcpLoopbackWorksUnderProviderHome`
- `TestTelegramMcpLoopbackDoesNotReadChildKeyring`
- `TestTelegramMcpLoopbackRequiresAutoApprove`
- `TestTelegramMcpDirectModeDisabledOrExplicitWhenRunnerEndpointMissing`
- `TestGrokTelegramLiveMCPServerIncludesLoopbackEnv`
- `TestEnsureGrokTelegramMcpConfigWritesLoopbackEnv`

## User action after rebuild

Settings → Telegram → **Configure Providers** so Grok/Codex/Claude/Gemini MCP entries pick up the new loopback env. Restart the FlowPilot runner so the route is mounted.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: BUG-281
change_type: bugfix
summary: Telegram MCP uses authenticated runner loopback so Grok HOME isolation no longer breaks keyring
# --->8---
