# CA-676 — TUI: keep mouse tracking ON on macOS/Linux (drag-copy was dead)

## Context

User (macOS Terminal.app): drag-select and Shift+click produced NO toast and
copied nothing — even after CA-674/675. Root cause was NOT the copy math (unit
tests call `handleMouse` directly and passed). The live TUI disabled mouse
tracking after Bubble Tea enabled it, so Terminal.app never delivered `MouseMsg`:

- `Init()` (`app.go:217`) ran `cmdEnsureMouseTrackingOff()` → wrote
  `ESC[?1000l?1002l?1003l?1006l`, turning tracking off on every platform.
- `maybeRearmConsoleInput` (`console_input_other.go:9`) re-sent mouse-off ANSI on
  every keystroke (non-Windows), killing tracking after the first keypress.
- `ClipboardPasteMsg` (`app.go:1516`) also called `ensureMouseTrackingOff()`.

Shift is only a modifier on a mouse event, so with no mouse events it did nothing.

## Changes

- `console_input.go`: added `shouldDisableMouseTracking() bool` = `GOOS=="windows"`
  and `initMouseCmd() tea.Cmd` (returns mouse-off cmd on Windows, else nil).
- `app.go` `Init()`: only attach `initMouseCmd()` when `shouldDisableMouseTracking()`.
  macOS/Linux keep `WithMouseCellMotion` tracking ON.
- `console_input_other.go` `maybeRearmConsoleInput` (non-Windows): no-op (no
  mouse-off ANSI) so tracking survives keystrokes.
- `app.go` `ClipboardPasteMsg`: `ensureMouseTrackingOff()` only on Windows.
- `Run()` log updated to report actual `mouseCellMotion` state.
- Windows behavior unchanged (BUG-328: tracking off, drag-copy unavailable).

## Tests (additive)

- `tui_program_opts_test.go`: `TestShouldDisableMouseTracking_WindowsOnly`,
  `TestInitMouseCmd_OnlyWindows`, `TestInit_SkipsMouseOffWhenTrackingWanted`.
- Full `go test ./internal/tui/app` green; drag/shift/click tests green.

## Verification (macOS Terminal.app, after rebuild)

- Drag a chat region → toast "Copied selection." + clipboard has the text.
- Shift+click a line → copies the whole line + toast.
- Typing stays live (no key wedge).
- Windows: mouse still off (drag-copy N/A, per BUG-328).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-311
change_type: bugfix
summary: Keep mouse tracking ON on macOS/Linux so drag/Shift-click auto-copy works
# --->8---
