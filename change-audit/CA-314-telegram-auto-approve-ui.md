# CA-314: Telegram auto-approve UI + clarify approval storage

## Context

CLI/Codex standalone testing of `flowpilot telegram-mcp` failed with:

`MCP_TOOL_APPROVAL_REQUIRED: sending Telegram messages requires auto-approve to be enabled for this integration`

The flag already existed on `telegramCredential.AutoApprove` (keyring) and on
`IntegrationConnectionRequest.TelegramAutoApprove`, but desktop Settings never
exposed a control, so every Connect saved `autoApprove: false`.

## What "approve" stores where

| Mechanism | When used | Storage |
|-----------|-----------|---------|
| **Auto-approve flag** | No run/step/process scope (Codex CLI, offline MCP) | Runner **keyring** credential JSON (`telegram:<integrationId>` → `{ botToken, channelId, autoApprove }`) |
| **Pending Approvals queue** | FlowPilot run with env scope set | Workspace file **`<workspace>/.flowpilot/telegram-proxy-approvals.json`** (pending → approved/rejected/executed) |
| UI mirror `autoApprove` | Display only on Existing Integrations | Supabase `integrations.config_encrypted.autoApprove` (non-secret boolean; not the secret boundary) |

Bot token never leaves the keyring path (SD-11 §6).

## Fix

1. **Runner**: `TelegramAutoApprove *bool` so plain re-Test (field omitted) does not wipe the flag; explicit pointer sets enable/disable without re-pasting bot token.
2. **Desktop Telegram MCP Settings**:
   - Create/Edit checkbox **Auto-approve sends (CLI / dev)**
   - Existing row button **Enable / Disable auto-approve**
   - Status line `auto-approve ON|OFF`
   - Pending Approvals help text documents the two stores
3. **client-core**: `testIntegration` fields accept `boolean` for `telegramAutoApprove`.
4. Test: `TestTriggerIntegrationConnectionTelegramAutoApproveToggle`.

## Source

- CP-05-05 / Task-233 approval model; live E2E CLI path for CP-05-04-05-06.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-05
change_type: feature
summary: Expose Telegram auto-approve in Settings and document keyring vs workspace approval stores
# --->8---
