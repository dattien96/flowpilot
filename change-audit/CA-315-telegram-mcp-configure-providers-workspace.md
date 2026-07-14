# CA-315: Telegram Configure Providers writes Drive-like launcher + workspace

## Problem

Grok CLI failed to connect `flowpilot_telegram` with:

`handshake failed: connection closed: initialize response` (exit ~20ms).

Root cause in **Configure Providers** write path (`Ensure*TelegramMcpConfig`):

1. Used `os.Executable()` → often `/var/folders/.../go-build.../exe/flowpilot` (ephemeral).
2. Args were only `['telegram-mcp']` — **no `--workspace`**, so when Grok spawns from a non-repo cwd the proxy cannot load `.flowpilot/mcp-backend-state.json` / resolve keyring secretKey.

Drive MCP already wrote the correct shape (`go -C apps/local-runner run ./cmd/flowpilot` + `--workspace`). Telegram did not.

## Fix

`telegram_mcp_provider_config.go`:

- `telegramProxyMcpCommand` / `telegramProxyMcpArgs` / `telegramProxyMcpInvocation` reuse Drive's launcher resolution.
- Claude / Codex / Gemini / Grok ensure + live merge all write:
  - command: `flowpilot` (PATH) or `go -C <ws>/apps/local-runner run ./cmd/flowpilot`
  - args: `telegram-mcp --workspace <abs workspace>`
  - Grok/Codex: `startup_timeout_sec=20`, `tool_timeout_sec=120`
- Staleness check requires `--workspace` present.

User action after rebuild: Settings → Telegram → **Configure Providers** (not hand-edit `config.toml`).

## Note

Keyring still must hold bot token (`secret not found in keyring` is a separate reconnect step). Config shape fix alone does not re-save secrets.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-05
change_type: bugfix
summary: Configure Providers writes Telegram MCP with stable launcher and --workspace like Drive
# --->8---
