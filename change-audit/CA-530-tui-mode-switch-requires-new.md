---
id: CA-530
feature_key: cli-tui
title: Require /new before switching modes while a run is open
date: 2026-08-17
status: COMPLETE
---

## Problem

`/flow <ref>` already blocked mode switches while a run was open
(`"Cannot change flow after a run has started. Use /new first."`), but `/chat`
had **no guard**: from a running flow you could `/chat` and silently drop the
flow mid-run (clearing the armed launch and switching the transcript context),
or from a running chat you could `/flow <ref>` into a live run. Both cases
should require `/new` first.

## Fix (TUI-only)

`/chat` in `app.go` now guards like `/flow <ref>` does:

```go
case "/chat":
    if m.runHandle != nil && m.mode != ModeChat {
        m.addMessage("system", "Cannot switch to chat while a run is open. Use /new first.", "error")
        break
    }
    …
```

Rules:
- Running (any mode) + `/chat` → blocked, system error points to `/new`.
- Running + `/flow <ref>` → already blocked (guard untouched).
- Armed flow, **no run** + `/chat` → still switches to chat and clears the
  launch (legacy `TestSlashChat_PersistsChatAndClearsFlow` parity).
- Same-mode `/chat` in a chat run → no-op as before.
- `/flow list`, `/new`, `/provider` behavior unchanged.

## Provider impact

Provider-agnostic (Case 1): the guard reads `m.runHandle`/`m.mode` only, no
`providerKey`. The blocked test runs × claude/codex/grok.

## Tests

New additive file `app/tui_mode_switch_requires_new_test.go`:

- `TestModeSwitch_ChatBlockedWhileRunOpen` — flow run open, `/chat` keeps
  ModeFlow + armed launch + runHandle, message hints `/new` × claude/codex/grok.
- `TestModeSwitch_FlowBlockedWhileRunOpen` — chat run open, `/flow pack/x` stays
  ModeChat, launch unarmed, message hints `/new`.
- `TestModeSwitch_ChatAllowedWhenNoRun` — armed flow with no run, `/chat`
  switches and clears launch (regression guard for legacy parity).

## Verification

- `go build ./...` clean; `go vet ./internal/tui/app/` clean.
- `gofmt` clean on changed files.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` — legacy
  `TestSlashChat_PersistsChatAndClearsFlow` untouched and green.
- `go test ./internal/tui/app -race -count=1` clean.

## Out of scope / residual

- `/new` still keeps the armed flow (user clears flow explicitly with `/chat`).
- `/flow list` read-only path intentionally not blocked.