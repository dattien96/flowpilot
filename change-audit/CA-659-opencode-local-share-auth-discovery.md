# CA-659 — OpenCode account discovery uses local share auth path

## Problem

TUI showed OpenCode as **installed · no account** even when `opencode` CLI was logged in.
Live verification on Windows: credentials live at `~\.local\share\opencode\auth.json`
(`opencode providers list` header), while FlowPilot only probed
`~\.config\opencode\auth.json` and did not recognize the provider-keyed credential JSON.

## Fix (auth paths)

- Resolve OpenCode auth in the same layered style as Grok/Codex:
  1. `OPENCODE_AUTH_PATH` (absolute or relative to data dir)
  2. `$XDG_DATA_HOME/opencode/auth.json` when set
  3. `~/.local/share/opencode/auth.json` (default on Linux, macOS, Windows)
  4. Platform fallbacks: `~/Library/Application Support/opencode/auth.json` (macOS),
     `%APPDATA%/opencode/auth.json` and `~/AppData/Roaming/opencode/auth.json` (Windows)
  5. Legacy `~/.config/opencode/auth.json`
- Validate OpenCode `auth.json` provider-keyed entries (`key`, `access`, `refresh`).
- Set `XDG_DATA_HOME` alongside `XDG_CONFIG_HOME` for managed/isolated OpenCode runs.

## Fix (account label + models)

- Account `display_label` / `account_name`: read connected providers from
  `auth.json` (Zen, Go, xAI) and optional `opencode providers list`; use xAI JWT
  email when present.
- Models: `GET /providers` now carries an 8s deadline so the adaptive probe
  budget (~6.5s) is real. Disk cache stores any successful live catalog (no
  10-model floor). Background warm is single-flight. Ambient `XDG_DATA_HOME` /
  `OPENCODE_AUTH_PATH` are honored for default-account discovery and launch.
  TUI retries `/providers` after 12s when OpenCode catalog is undersized.

## Tests

- `opencode_auth_detect_test.go` — XDG + platform auth paths.
- `opencode_account_models_test.go` — auth labels, model parse, cache, probe budget.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: OpenCode auth discovery, account provider labels, and live models cache/probe budget
# --->8---
