# CA-605: You-box rows are not wrapped in styleCanvas (stop Ghostty mid-pane wrap)

## What

Operator: copy returns the full 5-line prompt but the UI still showed fewer lines with the `│` border landing mid-pane (e.g. after `tra ve error`) and the dark canvas blob over the tail. `renderMessages()` (raw rows) was correct; the live `View()` was not. Also confirmed: no collapse/expand logic remains (CA-603) — the loss happened in the paint path.

## Why

- `renderChatPane` wrapped every transcript row in `styleCanvas.Render` (`48;2;13;13;13`) via `paintRow`. Ghostty / cellbuf count truecolor SGR (`38;2;R;G;B`) differently from plain rune counts, so the full-width You rows wrapped mid-pane and the wrapped tail was overwritten by the next row — hiding prompt lines.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go` — `isYouBoxRow(s)` (row starts with `┌`/`│`/`└`/`+`/`|`) and `padYouBoxRow(s, width)` (pad with a trailing canvas segment, no style on the glyphs).
- `apps/local-runner/internal/tui/app/app.go` `renderChatPane` — transcript rows that are You-box rows skip `paintRow`/`styleCanvas.Render` and use `padYouBoxRow`; the canvas fill behind them comes from `cellbuf.Fill` / the trailing pad segment. Composer/status/other rows keep `paintRow`.
- `youBox` unchanged (line-by-line prompt + dedicated `[copy]` row, CA-604).

## Tests

- `you_box_lines_test.go` `TestRegression_YouBoxUserPastedPromptAllLines` — now asserts on `m.View()` (the real paint path, incl. `composeCellBuf` when F2 is on), heights 24 + 30, widths 197/80 × claude/codex/grok: all 5 pasted lines present in order, no `error│` mid-word break at the box edge, `[copy]` NOT on the `symbols:` row.
- `go test ./internal/tui/app -count=1` 14s green; `go vet ./internal/tui/...` clean. Manual dump of `View()` at 197: all 5 lines + `[copy]` row, every `│` aligned, sidebar column clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: stop wrapping You-box rows in styleCanvas SGR so Ghostty no longer wraps the box mid-pane and hides prompt lines
# --->8---