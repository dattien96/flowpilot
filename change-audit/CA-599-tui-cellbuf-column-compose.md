# CA-599: Compose chat and sidebar via cell buffer rects (stop ANSI join bleed)

## What

You prompt box still bled into the F2 sidebar after string-level fixes (CA-579…598): `│ intent: …tra │` mid-screen with `Runner:` in the gap on Ghostty. Root cause: every approach composed the two panes by concatenating/padding ANSI strings on one line, so any width miscount of styled rows shifted the sidebar into the chat cells.

## Why

- `rasterClip`/`joinPanes` built `visualTakeStyled(chat) + visualTakeStyled(side)` — geometry depended on rune counting over ANSI-stripped strings and could still differ from the terminal's own width, especially with true-color SGR (`38;2;R;G;B`).
- `cellbuf.Render` (charmbracelet/x/cellbuf) trims trailing space cells (`strings.TrimRight(..., " ")`), which deletes the gutter between the chat column and the sidebar and shifts the sidebar one column left — the recurring "1-col bleed".
- OpenCode avoids this by painting cells (Yoga boxes + cell buffer), never by concatenating ANSI.

## Fix

- `apps/local-runner/internal/tui/app/cell_compose.go` (new) — `composeCellBuf(chat, side, fullW, chatW, sideW, h)`:
  - `cellbuf.NewBuffer(safeTermWidth(fullW), h)` (one spare column, CA-584), fill with canvas-bg cell (`ansi.HexColor(colorCanvas)`).
  - `cellbuf.SetContentRect(chat, Rect(0,0,chatW,h))` and `SetContentRect(side, Rect(chatW,0,sideW,h))` — chat cells can never occupy `x ≥ chatW`; sidebar cells are written at fixed positions, clipped by rect.
  - `renderGrid(buf)` serializes rows manually WITHOUT trimming trailing space cells (cellbuf.Render trims them), so the canvas gutter between chat and sidebar survives and every row is exactly `bufW` wide.
- `apps/local-runner/internal/tui/app/app.go:3929` — `View()` uses `composeCellBuf`; delete `rasterClip`/`visualTakeStyled` from `session_panel.go`.
- `go.mod` — `github.com/charmbracelet/x/cellbuf v0.0.15` now direct.

## Tests

- New `apps/local-runner/internal/tui/app/cellbuf_columns_test.go` — 250/197/160/120 × claude/codex/grok with the exact `[Change Contract]`/`intent: … tra ve error khi b > a`/`symbols: Subtract` prompt: chat cells (`x<chatW`) never contain `Runner:`/`Run: run-`/`session`/`Path: /tmp`; every line `≤ chatW+sideW` and `< terminalWidth`; You box borders at `≥ chatW-2`; sidebar content present.
- Full suite green: `go test ./internal/tui/app -count=1` 14.1s (137 tests incl. gutter/bar-align/raster-clip old guards), `go vet ./internal/tui/...` clean.
- Manual debug at 197: every row `len=196`, `│ [Change Contract]…│ Runner:` — all `│`/`┐`/`┘` at the same column, sidebar fixed at right.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: compose chat and F2 sidebar via cell buffer rects with no-trim render so styled rows cannot shift sidebar into chat
# --->8---