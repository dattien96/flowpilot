# CA-589: Left-align You box when F2 sidebar is on

## What

Reopening an old chat still showed the You box broken — `intent:`/`files:`/`symbols:` at col 0, frame on the right, `Runner:`/`Path:` inside the box. Pane isolation (CA-588) kept the two blocks separate, but the You box itself was still right-aligned, so its leading `[many spaces]│ intent…` wrapped on Ghostty and `intent:` fell to column 0.

## Why

- `strokeChatRows` always did `rightAlignPlain` for `user` boxes (gutter `width-2`). With `F2` sidebar on (`197` layout `chatW≈154`), the box hugged the join; Ghostty wrap of that leading space pushed content to the next line at col 0, exactly the screenshot when replaying a long prompt (`RunID` present when opening an old chat).
- `go test` compares `stripANSI(View)` strings, so `View` looked correct while the terminal composite wrapped.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go:219` — `strokeChatRows` now `func(..., user, ascii, alignRight bool)`. `rightAlignPlain` (top/inner/bottom) only when `user && alignRight`; otherwise the box stays left-aligned. `userBoxGutter` and `hugBoxWidth` unchanged.
- `apps/local-runner/internal/tui/app/app.go:4405` — call `strokeChatRows(..., true, m.asciiMode, !m.useRightSidebar())`. Sidebar on (wide + expanded + `hasContent`) → left; F2 collapsed or narrow `<100` → right as before.

## Tests

- New additive `apps/local-runner/internal/tui/app/you_box_left_when_sidebar_test.go:10` — `TestRegression_YouBoxLeftAlignedWhenSidebarOn` at 197 × claude/codex/grok with `RunID`: chat-only prefix (`≤chatW`) never contains `Runner:`, `┌ You` lead ≤8. `TestRegression_YouBoxRightAlignedWhenSidebarOff` at 80 keeps right-align.
- `go test ./internal/tui/app -count=1` 13.6s pass, `go vet` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: left-align You box when F2 sidebar is on so reopening a long chat does not wrap content to col 0
# --->8---
