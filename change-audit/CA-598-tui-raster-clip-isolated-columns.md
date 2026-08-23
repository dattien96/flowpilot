# CA-598: Raster-clip chat and sidebar into isolated columns

## What

You prompt box still bled into the F2 sidebar (`│ intent: …tra │` mid-screen with `Runner:` in the gap) even after pinning the inner bar to `chatW` (CA-596/597). The Go string was provably aligned (debug showed `│` at `chatW-2` == `┐` column) but Ghostty still misrendered because every row was one concatenated styled string — any SGR/width miscount moved the sidebar into the chat cells.

## Why

- `joinPanes` padded the chat string to `chatW` with `styleCanvas` spaces and appended the sidebar string: the two panes lived in the same ANSI line, so their geometry depended on Ghostty's reading of the styled chat row.
- Rows shorter than `chatW` (wrapped You rows, empty rows, status rows) were padded in `joinPanes` — but a `styleUser`-colored segment could still be counted differently by Ghostty, shifting the sidebar column.
- The user asked for hard separation ("chia width 2-8"): chat may only paint in the left `chatW` cells, sidebar only in the right `sideW` cells.

## Fix

- `apps/local-runner/internal/tui/app/session_panel.go:522` — new `visualTakeStyled(s, n, st)`: plain `[]rune(stripANSI(s))` width; if it fits, keep inner SGR and pad with `st` (CA-532 segment); if it exceeds `n`, clip on plain runes then `paintRow` with `st`. New `rasterClip(chat, side, chatW, sideW, h)`: rebuilds every row as `visualTakeStyled(chatRow, chatW, styleCanvas) + visualTakeStyled(sideRow, sideW, styleSidebar)` — chat can never occupy the sidebar cells and vice versa.
- `apps/local-runner/internal/tui/app/app.go:3929` — `View()` calls `rasterClip` instead of `joinPanes`.

## Tests

- New `apps/local-runner/internal/tui/app/raster_clip_test.go` — 250/197/160/120 × claude/codex/grok: every line `len ≤ chatW+sideW`; chat column (`rs[:chatW]`) never contains `Runner:`/`Run: run-`/`session`; You `┐`/`│` borders at `≥ chatW-2`; sidebar content present.
- Full `go test ./internal/tui/app -count=1` 14.4s pass (bg-preserving clip keeps `TestView_PaintsCanvasSidebarChatbarAndCode` green), `go vet` clean.
- Debug `View` at 197: `┌ You ─…┐` + `│ [Change Contract]…│ Runner:` all `│`/`┐`/`┘` at same column, sidebar `session`/`Runner:`/`Run:`/`Path:`/`steps` fixed at `chatW`, no mid-screen text.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: raster-clip chat and right sidebar into isolated columns so no styled-width miscount can bleed sidebar into the You box
# --->8---