# CA-594: Full-width You box when F2 sidebar is on

## What

Opening an old chat with a short prompt (`intent:`/`files:`/`symbols:`) showed three columns: You box hug-left ~60, gap, then `Runner:` middle, then `session` sidebar far right. AI/code boxes were full `chatW` and looked correct (two columns). Tests stayed green because they measured the Go string, not Ghostty.

## Why

- `hugBoxWidth` for `user` always did `7/10 * maxW` (60 at 154) even when `F2` was on. `renderChatPane` then padded that 60-col box to `chatW` with canvas spaces, but `paintRow`/`Width()` miscounted the styled `You` row on Ghostty, so the pad was short and `joinPanes` placed the 42-col sidebar right after the 60-col box — mid-screen, not at `chatW`.
- Go tests only checked `stripANSI(View)` strings, so the gap looked correct in strings while the terminal composite was short.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go:314` — `hugBoxWidth` now `func(..., fullPane bool)`. If `user && fullPane` return `maxW` directly (full chat pane, like the code box in image 1). `strokeChatRows` computes `fullPane = user && !alignRight` (`alignRight = !useRightSidebar()`), so `F2` on → full, `F2` off → hug 70% + right-align (keeps `TestRenderMessages_UserAlignedRight` 40-col).
- No change to `joinPanes`/`stripANSI`/`paintRow` (CA-592/593) — they already pad by `stripANSI` rune count; now the You row is already `chatW` before join, so the pad is just the 1-col safe gutter.

## Tests

- `go test ./internal/tui/app -count=1` 14.0s pass, `go vet` clean.
- Manual 197 F2 prompt: `┌ You` at 0, `│ intent:` inside, `┐` at `chatW-1` (153) + 1 canvas space, sidebar `session` at fixed `chatW`, no `Runner:` mid-screen.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: make You box full chat-pane width when F2 sidebar is on so narrow prompt does not pull sidebar into mid-screen
# --->8---
