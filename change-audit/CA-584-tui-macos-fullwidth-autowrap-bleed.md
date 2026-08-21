# CA-584: macOS full-width autowrap bleed (sidebar tràn vào chat)

## What

Sidebar content (`Runner:`, `Path:`) bled into the chat timeline on macOS Terminal.app / Ghostty / iTerm, but looked fine on Windows Terminal. You box itself was not wrapping wrong — the **whole View line** was `terminalWidth` wide, which autowraps on macOS and desyncs the frame.

## Why

- `joinRightSidebar` produced `contentW + sep + sideW = terminalWidth` (e.g. 197) and `paintRow(..., sideW, styleSidebar)` filled the last column. macOS terminals autowrap a row that fills the last column; `\n` then lands one line too low and the next frame's sidebar text paints over chat (`|Runner:`/`|Path:` inside timeline, You box appears glued — screenshot).
- Windows Terminal keeps the cursor at col `N` without wrapping, so `width==N` looked OK — hence "only macOS". `safeTermWidth` (last column empty) already existed but was only used for the composer/chat-bar, not for the final joined View.
- Wrap/gutter fixes (CA-579/582/583) kept fixing the You box's text, never the terminal line length, so "đã fix nhiều lần rồi mà vẫn bị" on macOS.
- Old tests asserted `Width <= w` (equal allowed), so CI stayed green.

## Fix

- `apps/local-runner/internal/tui/app/session_panel.go:506` — `joinRightSidebar` now caps the combined line to `safeTermWidth(fullW)` (`fullW-1`). Keep chat at `contentW`, shave the last column off the **sidebar paint** (`sideW-1`) so total is `fullW-1` instead of `fullW`. The rightmost terminal column stays default bg (one empty column) on every platform; chat and You box keep their 2-column gutter (CA-583).
- `apps/local-runner/internal/tui/app/app.go:3892` comment updated; non-sidebar rule lines intentionally stay at full width (legacy test `80 "-"` rule) — only the sidebar-joined View is shortened, which is the only path that desynced on macOS.
- No GOOS fork; one code path for macOS + Windows.

## Tests

- New additive `apps/local-runner/internal/tui/app/view_no_fullwidth_wrap_test.go:14` — `TestRegression_ViewNeverFillsLastColumn` at 197/120 with sidebar and 80 without. Every `View` line `Width < terminalWidth` when sidebar is active (strict `<`), `<=` otherwise — proves macOS autowrap cannot trigger. Also verifies You box still has gutter.
- Existing suite still green (`go test ./internal/tui/app -count=1` 14s pass; previous `TestView_PadsBlankLineAndRuleBeforeStatus` / `TestView_ChatSeparatedFromStatusByRule` keep `80` rule). Manual `View` debug at 197: `vw=196` for every line (was `197`), e.g. `┌ You ─...┐   │session` + `│ ... [copy]│   │Runner:`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: cap sidebar-joined View to terminalWidth-1 so macOS Terminal does not autowrap sidebar into chat
# --->8---
