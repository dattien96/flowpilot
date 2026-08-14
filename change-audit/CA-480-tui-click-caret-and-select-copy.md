---
id: CA-480
feature_key: cli-tui
title: Click-to-place prompt caret; Ctrl-C copies drag selection
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-478-tui-drag-select-and-input-cursor
will_not_undo: CA-478 plain drag-select + arrow caret; CA-451 idle Ctrl-C quits; running Ctrl-C stops
```

## Issue

After CA-478 (drag-select without Shift + arrow caret):

1. Prompt caret only moved via keys; operators wanted click-to-place on Win and macOS.
2. Drag highlight + Ctrl-C still quit the TUI (mouse tracking blocks native terminal copy).

## Change

- Left press on the prompt body maps cell coords to a rune caret (`tryPlaceInputCursor`), matching `renderInputLine` layout (frame pad, `[+img]`, horizontal window). Attach/Approve/Deny chips are excluded. Windows `Type=MouseLeft` (Action=0) uses the same path as Press.
- Ctrl-C with an armed `mouseSel` copies `selectionPlainText()` (ANSI-stripped visible rows under the highlight) via `cmdCopyText`, clears the selection, and does **not** quit. Empty selection text is a no-op status. Idle Ctrl-C still quits; running Ctrl-C still stops the turn when nothing is selected.

TUI chrome only. Clipboard write reuses existing Win/macOS paths.

## Tests

Additive `tui_click_caret_select_copy_test.go`. Full `./internal/tui/app` green; pre-existing tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI click places prompt caret; Ctrl-C copies drag selection instead of exit
# --->8---
