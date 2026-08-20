# CA-579: Fix You prompt box overflow and right sidebar overlap

## What

Long user prompts (with `|Path:` pipes, long paths, Vietnamese) overflowed the `You` box and the right sidebar (`session`/`steps`) overlapped the chat column. Added wrapping/clamping so the box always stays within `chatWidth` and `rightAlignPlain` never exceeds width.

## Why

- `wrapParagraph` used rune count (`len(runes)`) not visual width, and `hugBoxWidth`/`rightAlignPlain` could return strings wider than `chatWidth`/`contentWidth`. When `useRightSidebar` is true (`>=100` cols, F2 expanded), `buildChatRows` width is `contentWidth = terminalWidth - sideWidth -1`, but `strokeChatRows` right-aligned a box wider than `contentWidth`, so the box's right border `│` landed on the sidebar separator `│` (`││` in screenshot) and content `|Path:` spilled.
- Clamped prompt (`max 4 lines + "...." + [copy]`) appended `....` to a full-width wrapped line without reserving space, making the last line `contentWidth+4` and expanding `hugBoxWidth` beyond `capW`, again pushing the border into the sidebar.

## Fix

- `apps/local-runner/internal/tui/app/session_panel.go:350` — `rightAlignPlain` now truncates with `truncateVisual` when `w > width` (was returning overflow as-is).
- `apps/local-runner/internal/tui/app/chat_box.go:261` — `hugBoxWidth` uses `lipgloss.Width` for title and keeps `+1` for the leading `" " + r.Text` so `innerW` matches the later `strokeChatRows` `text := " " + r.Text` padding.
- `apps/local-runner/internal/tui/app/helpers.go:662` — `wrapParagraph` early checks `lipgloss.Width(para) <= width` and `lipgloss.Width(string(runes)) <= width`; keeps whitespace-break logic but now on visual width.
- `apps/local-runner/internal/tui/app/app.go:4195` — when `userTruncated` on the last line, cut the rendered line to `contentWidth - ellipsisW` visual width without adding an extra `…`, then append `styleStatus.Render("....")` so `last line + .... + [copy]` still fits `contentWidth` and `hugBoxWidth` stays ≤ `capW`.
- `apps/local-runner/internal/tui/app/prompt_overflow_test.go` (new, additive) — regression for pipe-heavy, no-spaces path, Vietnamese, very-long prompts at 60/80/100/120 cols with and without right sidebar; plus clamp still shows `....` and `[copy]` without overflow. All widths checked against `chatWidth` and `m.View()` against terminal width.

## Tests

- `go vet ./internal/tui/...` clean (other than pre-existing `gitnexus.go`).
- `go test ./internal/tui/app -run TestRegression_UserPrompt -count=1` 2/2 pass (17 subcases).
- `go test ./internal/tui/app -count=1` 13.5s green, including `TestBuildChatRows_DoesNotAssume80WhenNarrow`, `TestView_NarrowWidthDoesNotOverflowInputRows`, and existing `TestUserPrompt_*` clamp tests.

## Residual

- Box remains hug-width and right-aligned (`width*7/10` cap) as before; no change to Desktop or composer.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: fix You prompt box overflow and right sidebar overlap by clamping right-align, visual-width wrapping, and reserving ellipsis width
# --->8---
