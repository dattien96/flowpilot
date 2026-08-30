# CA-658 — Show Not installed instead of Node module dumps for broken CLIs

## What

TUI `/provider` showed Codex as `Error: Cannot find module …` (npm shim `--version` stderr). Operator asked for red **Not installed** instead of the dump.

## Why

`resolveVersion` stored CombinedOutput as the version string whenever stdout/stderr was non-empty, even on failure. The picker appended that dump next to the provider name.

## Fix

- Runner: `looksLikeCLIProbeError` rejects cannot-find-module / MODULE_NOT_FOUND / multi-line dumps; `resolveVersion` returns "".
- TUI: FAILED install or probe-error version → `not_installed`. Picker/list show `not installed` / `Not installed`, no dump. Those rows render with `styleError` (red).
- `/provider install` is allowed when the CLI is unusable (FAILED / module dump) even if a PATH shim still exists — runner already skips only `InstallStatus==INSTALLED`. Healthy installs still say already installed.

Codex/Claude/Grok/OpenCode share the same detect+picker path (provider-agnostic sanitizer).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Hide Node module dumps on provider picker; show red Not installed
# --->8---
