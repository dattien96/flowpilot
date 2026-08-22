# CA-591: Pin chat pane width before sidebar join

## What

You prompt box still bled into the F2 sidebar on some scroll positions: when the narrow hug-left You box (~60 cols) was joined raw, `JoinHorizontal` trimmed trailing spaces and the next line's `Runner:`/`Path:` appeared mid-screen, glued to the box's right edge. Wide AI/code boxes filled `chatW` so the join landed at the right edge and looked correct.

## Why

- `renderChatPane` returned `strings.Join(rows)` without `Width(chatW)` when isolated (CA-588 removed outer Width to keep `barW = w-1` gutter for narrow test). `JoinHorizontal` then used each line's raw width: a 60-col You line plus 42-col sidebar became `102`-wide joined row, not `chatW+sideW`, so sidebar started mid-screen on narrow You rows but at the right edge on wide AI rows — the column visibly jumped.
- Existing `view_no_fullwidth`/`gutter` tests only checked `Width < term` and trailing space, not fixed column.

## Fix

- `apps/local-runner/internal/tui/app/app.go:3924` — `View()` now does `chatRaw := renderChatPane(chatW,h); if useRightSidebar() { chat = Width(chatW).MaxWidth(chatW).Render(chatRaw); side := renderSidebarPane(sideW,h); chat = JoinHorizontal(Top,chat,side) }`. The Width pin is only when sidebar is active — narrow no-sidebar (28 cols) keeps `barW` gutter and existing `TestView_NarrowWidthDoesNotOverflowInputRows` stays green.
- No change to `strokeChatRows` left-align (CA-589), `?7l`/`width-2`/`paintRow` measure-after-Render.

## Tests

- New additive `apps/local-runner/internal/tui/app/you_box_sidebar_column_test.go:10` — 197/160/120 × claude/codex/grok with prompt + sidebar: `Runner:`/`session` never starts at `<chatW-1` on a `You`/`intent:` row, gap ≥20 cols, every View line `< term`.
- `go test ./internal/tui/app -count=1` 13.7s pass, `go vet` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: pin chat pane to chatW before JoinHorizontal so narrow You box does not pull sidebar into mid-screen
# --->8---
