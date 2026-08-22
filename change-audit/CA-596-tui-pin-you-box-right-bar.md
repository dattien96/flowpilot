# CA-596: Pin You box right bar to chatW with plain geometry

## What

Image 1 still showed `┌ You` wide (`session` column) but inner rows `intent: …tra |Runner:` with `│` mid-screen and `Runner:` inside the box. AI/code rows were full-pane and correct — only the short hug You rows leaked.

## Why

- `hugBoxWidth` hug 70% even when `F2` on, but `CA-594` made the box full-pane (`maxW`) only for the integer width, not for the styled inner. `padVisualANSI` and `strokeLine` measured `lipgloss.Width(styled)` for `styleUser` (`38;2;R;G;B` true-color with `:`). Ghostty counted differently, so a 60-col `intent: …tra` was considered 151 and not padded; the `│` landed at text end, not at `chatW-1`, and `Runner:` (sidebar at fixed `chatW`) appeared right after `tra` inside the gap.
- `chatRowsSig` did not include `useRightSidebar`, so a cache built before `F2` populated could be reused after, keeping the hug width.
- Tests compared `stripANSI(View)` strings, so the 1-column `lipgloss` vs Ghostty delta never failed.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go:146` — `paintRow` measures `len([]rune(stripANSI(...)))` before/after `Render`, not `lipgloss.Width(styled)`.
- `apps/local-runner/internal/tui/app/mouse.go:13` — `stripANSI` `\\x1b\\[[0-9:;?]*[ -/]*[@-~]|\\x1b\\][^\\x07]*\\x07|\\x1b\\(B` to cover `:` true-color and OSC.
- `apps/local-runner/internal/tui/app/chat_box.go:314` — `hugBoxWidth(..., fullPane bool)`; `user && fullPane` returns `maxW` (full pane). `strokeChatRows` computes `fullPane = user && !alignRight` (`!useRightSidebar`).
- `apps/local-runner/internal/tui/app/chat_box.go:298` — when `fullPane`, rebuild inner rows with plain geometry: `base = " " + stripANSI(r.Text)` padded to `innerW` (and `innerW-chip` when `[copy]`), then `line = "│" + styleUser.Render(base) + "│"` so the right `│` is always at `boxW-1` (`chatW-1` after `safeTermWidth`), independent of styled width.
- `apps/local-runner/internal/tui/app/app.go:4290` — `isFullPaneBox = boxed && useRightSidebar()` forces `rightAlign=false` and `contentWidth = width-4-copyChip` (full inner) before `wrapText`, so wrap and box agree (`tra ve` no longer split mid-word).
- `apps/local-runner/internal/tui/app/app.go:4114` — `chatRowsSig` now hashes `useRightSidebar()` so the cache invalidates when `F2` toggles or an old chat is reopened.

## Tests

- Existing `change_contract_prompt_box_test` now shows `intent: …tra ve error` on one line inside a full `│ … │` at 197 F2 on (was `tra│` / `│ ve error`), and two lines at 80. No `tra│` in plain.
- `go test ./internal/tui/app -count=1` 14.0s pass, `go vet` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: pin You box right bar to chatW with plain geometry so styled width mismatch does not pull sidebar into box
# --->8---
