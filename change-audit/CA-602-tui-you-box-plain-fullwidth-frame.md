# CA-602: You box = one plain full-width text frame (drop styled stroke pipeline)

## What

Operator: "chỉ fix show prompt box thôi mà sao khó vậy. 1 text show full width là xong rồi". After CA-601 the box chrome was full width but the inner rows still mis-rendered on Ghostty: the `│` border and `[copy]` chip stopped mid-row with a dark canvas blob after `tra ve error`.

## Why

`strokeChatRows` still carried a re-wrap + ANSI-aware padding pipeline from the hug-era (CA-593/595/600): it re-wrapped styled rows and padded by `len(stripANSI(...))`, which differed from Ghostty's true-color SGR width count. Any per-row width guess can drift — the box must not depend on ANSI geometry at all.

## Fix

- `apps/local-runner/internal/tui/app/chat_box.go` — replaced `strokeChatRows` with `youBox(lines, width, ascii, copyOn, msgIdx, expandKey) []chatRow`: draws `┌ You ─…┐`, one row per pre-wrapped line as `│` + plain rune body + `│` (pad or hard-slice to `innerW`), `[copy]` on the last row flush before the border, `└─…┘`. No styled spans, no ANSI-aware pad, no re-wrap.
- `apps/local-runner/internal/tui/app/app.go` — boxed-user branch wraps `wrapText(msg.Content, width-3)`, clamps to 4 lines + a `....` row (rided with `[copy]`, matching old rewrap output), then calls `youBox` directly.

## Tests

- `go test ./internal/tui/app -count=1` 14.2s green (full-width, line-by-line, gutter, cellbuf, clamp/ellipsis, overflow guards all pass); `go vet ./internal/tui/...` clean.
- Manual dump at 120/197: every row exactly `width-1` runes, `[copy]` flush before right `│`, no blob.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: replace styled stroke pipeline with a single plain full-width You box frame so borders and copy chip never drift
# --->8---