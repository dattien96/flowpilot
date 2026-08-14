---
id: CA-486
feature_key: cli-tui
title: Fix prompt caret after backspace/clear (abc → bca)
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-478-tui-drag-select-and-input-cursor, CA-480-tui-click-caret-and-select-copy
will_not_undo: left/right/home/end; sticky-end inputCursor==-1 contract
```

## Issue

After clearing the prompt (backspace or Esc) and typing again, the caret jumped
in front of the last character so `abc` became `bca`.

## Root cause

`inputCursor == -1` means sticky end. After backspace, `deleteInputBeforeCursor`
called `moveInputCursor(-1)` on the *already shortened* string: sticky caret
resolved to `newLen`, then −1 → index before the remaining rune.

Esc/`inputValue=""` also left a stale mid-string `inputCursor` until clamped.

## Fix

- `insertInputAtCursor` / `deleteInputBeforeCursor` set caret from known indices
  (`setInputCaret`); sticky-end backspace stays sticky.
- `clearInputValue()` resets value + caret for Esc / Enter / picker paths.

## Tests

Additive `input_cursor_sticky_end_test.go`. Prior arrow/home tests green.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Keep TUI prompt caret sticky-end after backspace and clear
# --->8---
