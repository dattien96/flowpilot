# CA-592: Join chat and sidebar by stripANSI pad (Ghostty Width mismatch)

## What

After pinning chat to `chatW` with `Width(chatW).Render` before `JoinHorizontal` (CA-591), image 1 still showed `Runner:` in the middle of the left-aligned You box, and sidebar `session` on the right — three columns. `Width().Render` did not pad the narrow You rows on Ghostty.

## Why

- `lipgloss.Width(styled)` for `styleUser` You rows counts differently than Ghostty's visual columns, so `Width(chatW).Render` thought a `You` line was already `chatW` and added no padding. `JoinHorizontal` then trimmed trailing ANSI spaces and placed the 42-col sidebar right after the 60-col box, not at `chatW`. Wide AI/code rows filled `chatW` so the sidebar landed at the far right and looked correct — only the narrow hug-left prompt leaked.
- All Go tests compare `stripANSI(View)` or `lipgloss.Width` on the same styled string, so they stayed green while the terminal composite was wrong.

## Fix

- `apps/local-runner/internal/tui/app/session_panel.go:502` — add `joinPanes(chat,side,chatW,sideW,h)`: split both blocks to `h` rows, truncate chat rows to `chatW` via `truncateVisual` on `stripANSI` width, then pad chat rows to exactly `chatW` with `styleCanvas` spaces (visual, not `lipgloss.Width(styled)`). Concatenate `chatPadded + sideLine` per row.
- `apps/local-runner/internal/tui/app/app.go:3924` — `View()` now `chatRaw := renderChatPane(chatW,h); if useRightSidebar() { side := renderSidebarPane(sideW,h); chat = joinPanes(chatRaw,side,chatW,sideW,h) }`. Remove `Width(chatW).Render` + `JoinHorizontal`. Narrow no-sidebar path (28 cols) is untouched, so `barW = w-1` gutter stays.

## Tests

- Existing `you_box_sidebar_column_test.go` still green (now via stripANSI column). Full `go test ./internal/tui/app -count=1` 13.9s pass, `go vet` clean.
- New manual check: 197 F2 prompt `intent:` row has `Runner:` at `>=chatW-1` (fixed column), not mid-screen.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: join chat and right sidebar by visual stripANSI pad so Ghostty width mismatch does not pull sidebar into mid-screen
# --->8---
