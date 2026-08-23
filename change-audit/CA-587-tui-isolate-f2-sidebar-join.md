# CA-587: Isolate F2 sidebar via JoinHorizontal (stop bleed into chat)

## What

Right sidebar was not isolated — `joinRightSidebar` did `l + sep + s` per line. A long right-aligned You bubble filling `contentW` and a macOS wrap left `Runner:`/`Path:` painting over the chat frame (screenshot: `intent:`/`files:`/`symbols:` at col 0, You frame on the right, `Runner:`/`Run:`/`Path:` inside the box).

## Why

- Chat and sidebar were one concatenated string per row. `padTo`/`truncateVisual` on `l` kept it at `contentW`, but any terminal wrap or ghost `width-1`/`?7l` miss let the terminal's cursor land in the left column and the next `sep+side` chunk rendered over chat. `go test` compares `stripANSI(View)` strings, so the string looked correct while the terminal composite was wrong.
- Previous fixes (CA-584 `width-1`, CA-585 `?7l`, CA-586 `width-2` re-anchor) narrowed the total width but never separated the two rectangles.

## Fix

- `apps/local-runner/internal/tui/app/session_panel.go:501` — rewrite `joinRightSidebar` to isolate rectangles via `lipgloss.JoinHorizontal`. Pad both sides to `n = max(len(left), len(side))` rows. Pre-truncate left lines to `contentW` (visual). Build `leftCol` (`Width/MaxWidth=contentW`), `sepCol` (`Width=1`, `Background=colorBg2`), `rightCol` (`Width/MaxWidth=sidePaintW`, rows pre-painted with `styleSidebar`). `sidePaintW = sideW - (fullW-safeFull)` keeps total at `safeTermWidth(fullW)` (CA-584). Join with `lipgloss.JoinHorizontal(lipgloss.Top, leftCol, sepCol, rightCol)` so neither column can flow into the other.

## Tests

- `go test ./internal/tui/app -count=1` 13.9s pass, `go vet` clean.
- Manual `View` at 197 F2: `vw=196` (`Width < 197`), left `contentW=154`, You box `┌ You ─...┐   │` with gutter, no `Runner:` in left block.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: isolate F2 sidebar via JoinHorizontal so chat and sidebar cannot bleed into each other
# --->8---
