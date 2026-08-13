---
id: CA-478
feature_key: cli-tui
title: Drag-select without Shift; left/right edit the prompt
date: 2026-08-14
status: COMPLETE
---

## Change

TUI transcript select no longer requires Shift. Left press on empty chrome
remembers the origin; motion arms the highlight; release after a drag keeps
it. A press with no motion is still a click (clears highlight, fires
Approve/copy chips). Shift+drag is unchanged. CA-457 click-to-clear stays.

The chat prompt keeps an optional rune cursor (`inputCursor < 0` = end, so
legacy tests that only set `inputValue` still append). Left/Right/Home/End
move it; type and Backspace edit at the caret.

TUI chrome only. No runner or Desktop path.

## Provider impact

Provider-agnostic.

## Tests

New `tui_input_cursor_select_test.go`. Old mouse/click/input tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: TUI drag-select without Shift and left/right prompt cursor
# --->8---
