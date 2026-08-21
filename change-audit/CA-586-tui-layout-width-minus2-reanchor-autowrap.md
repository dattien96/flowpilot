# CA-586: Layout width-2 and re-disable autowrap after resize (macOS bleed still)

## What

After kill+restart the You box was still exactly as in screenshot (`intent:`/`files:`/`symbols:` at col 0, frame `You` on the right, `Runner:`/`Path:` inside the box, sidebar missing Runner/Path). The previous `?7l` once in `Init` did not stick — ClearScreen/alt-screen in Ghostty/OpenCode re-enables autowrap, and the first frame was already drawn while wrap was still on.

## Why

- View string in Go was correct, but the terminal's DECAWM was on when the first frame painted. Right-aligned You rows (`[many spaces]│ intent…│ …│Runner:`) wrapped: the leading spaces filled row 1, content fell to row 2 col 0, leaving the ragged overlay seen in image. Windows Terminal does not wrap at the last column, so it looked OK.
- `safeTermWidth`/`gutter`/`join width-1` only fix the string; they do not keep the terminal from wrapping once autowrap is back on. A single `?7l` in `Init` races the renderer and is clobbered by the next `ClearScreen`/resize.
- `WindowSizeMsg` reported 197 from the PTY, but OpenCode's pane (scrollbar/padding/border) is 1–2 columns narrower inside Ghostty — the string was 2 columns too wide for the drawable area.

## Fix

- `apps/local-runner/internal/tui/app/app.go:211` — `WindowSizeMsg` now stores `w = msg.Width - 2` (guard `w < 40 → keep msg.Width`) into `m.width`/`m.fullWidth`, so a 197 PTY lays out as 195. Keeps the 2-column PTY/pane delta out of the layout.
- `apps/local-runner/internal/tui/app/app.go:211` — return `tea.Batch(tea.ClearScreen, cmdSetAutoWrap(false))` instead of `tea.ClearScreen` alone, so `?7l` is resent after every resize/ClearScreen.
- Prior CA-584 join cap (`width-1`) and CA-585 `?7l`/`paintRow` measure-after-Render are kept; this is strictly the terminal re-anchor layer.

## Tests

- Existing `WindowSizeMsg` responsive tests keep passing: 36 and 40 stay unchanged (guard), 120/200 map to 118/198 and the View stays renderable.
- `go test ./internal/tui/app -count=1` 14.3s pass, `go vet` clean.
- `view_no_fullwidth_wrap_test.go` still passes: 197 with sidebar now produces `vw=194 < 197` (`195` layout → `194` safe), 80 no-sidebar stays `<=80`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: layout width-2 and re-disable autowrap after resize to stop macOS Ghostty desync reappearing after kill/restart
# --->8---
