# CA-583: Keep You bubble gutter before F2 sidebar (no more ││ glue)

## What

You bubble was right-aligned flush to the F2 sidebar separator (`┐│session` / `││Runner` in the 197×57 screenshot). The box kept 1 column from `safeTermWidth` — not enough to separate the bubble's `│`/`┐` from the sidebar's `│`. Add a 2-column gutter so the bubble visibly stops before the sidebar.

## Why

- Prior fixes (CA-579 wrap, CA-582 re-wrap) fixed string-over-width and mid-box truncation, but not placement. `hugBoxWidth` capped at `maxW*7/10` and `strokeChatRows` did `rightAlignPlain(..., width)` (= `chatWidth-1`) — the bubble's right edge landed exactly at `contentW` and the sidebar sep `│` followed immediately. Tests only asserted `width ≤ chatWidth`, so flush still passed; no test at 197×57 existed.
- Screenshot proof: user branch tip on 197×57 still showed `┐│session` / `││Runner`.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go:303` — `const userBoxGutter = 2`. `hugBoxWidth` for user uses `effW = maxW - gutter` to compute the `7/10` cap so the hug doesn't fill the gutter. `strokeChatRows` computes `alignW = width - gutter` (fallback to `width` if <10) and right-aligns every user line (top/inner/bottom) to `alignW` instead of `width`. After `View` → `paintRow(..., chatWidth)` the left column ends with `gutter+1` spaces (2 + safeTermWidth) before the sidebar sep: `┐   │session`.

No change to `joinRightSidebar` truncate (avoids regressing other rows), no change to wrap logic.

## Tests

- New additive `apps/local-runner/internal/tui/app/you_box_sidebar_gutter_test.go:14` — `TestRegression_YouBoxKeepsGutterBeforeSidebar` at 197/160/140/120 × claude/codex/grok (F2 expanded). For each View line carrying a You box: chat column (before `sideX-1`) ends with a space (gutter), does not contain `Runner:`/`session`, retains its right border (`│`/`┐`/`┘`), and View line `≤ terminalWidth`.
- `go test ./internal/tui/app -count=1` 13.5s green; `go vet ./internal/tui/...` clean (pre-existing gitnexus vet warn only).
- Manual `View` debug at 197: `┌ You ─...┐   │session` / `│ ... [copy]│   │Runner` — 3 spaces (2 gutter + 1 safeTermWidth) instead of 1.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: keep You bubble gutter before F2 sidebar so box border no longer glues to sidebar separator
# --->8---
