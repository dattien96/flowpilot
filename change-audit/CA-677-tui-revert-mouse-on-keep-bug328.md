# CA-677 — TUI: revert mouse-on, keep BUG-328 (no hang regression)

## Context

CA-674 re-enabled `tea.WithMouseCellMotion` in `tuiProgramOpts` on **every**
platform to restore drag/Shift-click auto-copy + "Copied" toast. CA-676 then
gated the mouse-OFF ANSI behind `GOOS=="windows"` so macOS/Linux kept tracking on.
But BUG-328 turned mouse tracking OFF specifically because application mouse mode
(`ENABLE_MOUSE_INPUT` + `?1002h`) wedges keyboard input (Windows conhost focus
steal; the spec disables it on all platforms for matching UX). Re-enabling it —
even only off-Windows — reopens the hang risk class and contradicts BUG-328
`D-1`/`D-6`. User decision: revert mouse-ON, stay BUG-328-safe.

Copy path (per BUG-328 `D-6`): terminal **native bôi-đen** + `/copy`. The app does
NOT receive mouse events, so there is no in-app "Copied" toast — that is expected.

## Changes (revert CA-674 opts + CA-676 gating)

- `app.go` `tuiProgramOpts`: back to `{WithAltScreen, WithFilter(tuiMsgFilter)}`
  only — `WithMouseCellMotion` removed on all platforms.
- `app.go` `Init()`: always `cmdEnsureMouseTrackingOff()` (every OS), restored.
- `app.go` `ClipboardPasteMsg`: `ensureMouseTrackingOff()` unconditional, restored.
- `console_input.go`: removed `shouldDisableMouseTracking()` / `initMouseCmd()`
  helpers (no longer needed).
- `console_input_other.go` `maybeRearmConsoleInput` (!windows): re-sends
  mouse-off ANSI (defense-in-depth, restored to BUG-328 behavior).
- `app.go` Run() log updated to "mouseCellMotion=off (BUG-328)".
- Tests reverted to len==2: `tui_program_opts_test.go`, `bug328_program_opts_test.go`,
  `ca610_tui_input_live_test.go`; removed CA-676 darwin-only tests.

## Kept (no-op now, harmless)

- CA-674 UX: removed F2/F3 hint line and trailing `[copy]` chips (unchanged).
- CA-675 `handleMouse` Shift/drag logic: dead while mouse is off, but kept so a
  future opt-in mouse mode works without re-implementing it. `tui_drag_copy_test.go`
  still passes (calls `handleMouse` directly).

## Verification

- `go test ./internal/tui/app -count=1` green (7.3s). Program-opts tests assert 2 opts.
- Manual (macOS Terminal.app): mouse tracking OFF → native bôi-đen copies via the
  host (Cmd+C / copy-on-select). Keyboard stays live (no wedge). No in-app toast.
- Windows: unchanged (BUG-328), drag-copy unavailable; `/copy` fallback remains.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-311
change_type: bugfix
summary: Revert mouse tracking ON; keep BUG-328 hang-safe, copy via native selection + /copy
# --->8---
