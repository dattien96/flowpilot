# CA-609: expand tabs in code fence blocks to align box border strokes (TUI)

## What

Operator reported: "ô coding block thif content ok mà cái char của stroke box hơi lệch" — The screenshot showed a Go code block (`media_1787520267345.png`) where lines with tab indentation (`\tif b > a {`, `\t\treturn 0, ErrSubtrahendTooLarge`) caused the right vertical border stroke (`│`) to shift to the right, creating a jagged stair-step / wedge misalignment.

## Why

- In terminal rendering, a tab character (`\t`) advances the cursor to the next tab stop (4 or 8 columns).
- `renderCodeFenceBox` in `apps/local-runner/internal/tui/app/markdown_render.go` previously counted `\t` as 1 rune (`len([]rune(line))`), and `padVisualANSI` padded the line assuming `\t` occupies only 1 column.
- When the terminal expanded `\t` to multiple columns, the padded content exceeded `padW`, pushing the right border `│` outward on indented lines.

## Fix

- `apps/local-runner/internal/tui/app/markdown_render.go`:
  - Added `expandTabs(s string, tabWidth int) string` to expand tab stops to 4 spaces.
  - In `renderCodeFenceBox`, expanded tabs in all body lines before calculating visual inner width (`lipgloss.Width(expanded)`) and wrapping.
- `apps/local-runner/internal/tui/app/chat_box.go`:
  - Updated `padVisualANSI` to use `lipgloss.Width(stripANSI(s))` instead of `len([]rune(stripANSI(s)))` so visual column widths for ANSI strings and wide characters are measured accurately.

## Tests

- Added `apps/local-runner/internal/tui/app/markdown_code_tab_align_test.go`:
  - `TestRenderMarkdown_CodeBlockTabIndentationBordersAligned`: verifies exact Go code snippet from user screenshot with 1-tab and 2-tab indentations produces perfectly aligned right borders (`│`) across multiple terminal widths (60, 80, 100, 120) in both Unicode and ASCII box modes.
  - `TestExpandTabs_TabStops`: verifies tab stop calculations for leading tabs, double tabs, and mid-line tabs.
- All existing markdown and prompt tests pass.

## Provider parity

Provider-agnostic markdown rendering in TUI.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: expand tabs in code fence blocks so right border strokes align vertically without tab-indentation drift
# --->8---
