---
id: CA-444
feature_key: cli-tui
title: Fix desktop auth save race condition and persist TUI session provider/model defaults
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-443-cli-tui-initial-implementation
will_not_undo: CP-46 Grok ACP integration
```

## Summary

1. Fixed desktop-flowpilot Electron main process `savePersistedAuthSession` `ENOENT` error by using a unique temporary file per atomic write.
2. Updated TUI `SessionDefaultsMsg` handler to automatically persist resolved `provider` and `model` to `tui-session.json` on disk via `persistTUISessionPrefs`.
3. Verified all TUI and CLI tests pass 100%.

## Changes Made

1. **apps/desktop-flowpilot/electron/main.ts**: Updated `savePersistedAuthSession` to write to a unique temporary file (`${dest}.${Date.now()}.${rand}.tmp`).
2. **apps/local-runner/internal/tui/app/app.go**: Added `persistTUISessionPrefs(m.provider, m.model, m.reasoningEffort)` inside `SessionDefaultsMsg`.

## Verification

- `go test ./internal/tui/... ./internal/cli/...` passed 100%.
