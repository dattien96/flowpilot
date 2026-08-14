---
id: CA-481
feature_key: cli-tui
title: Character-level transcript drag selection
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-480-tui-click-caret-and-select-copy
will_not_undo: CA-478 plain drag; CA-480 Ctrl-C copies selection
```

## Issue

Drag-select painted and copied whole lines even when the mouse only spanned a few cells.

## Change

`applyMouseSelection` and `selectionPlainText` use inclusive display-column ranges from `(x0,y0)`→`(x1,y1)`:

- same row: cells `[x0, x1]`
- multi-row: first row x0→EOL, middle full, last BOL→x1

Wide runes use `lipgloss.Width`. Ctrl-C copy inherits the same slice.

## Tests

Additive `tui_char_select_test.go`. Full `./internal/tui/app` green.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI drag-select highlights and copies character columns not whole lines
# --->8---
