# CA-601: You box renders prompt lines plainly, line by line, chip flush right

## What

After CA-600 (full-width You box) the box chrome was full width but the inner content was still broken: the `│` border and `[copy]` chip stopped mid-row with a dark canvas blob after `tra ve`, and lines wrapped one column early (`…khi b >` / `a`). Operator: "giờ full width rồi thì show line by line như bình thường".

## Why

- Inner rows were still `paintWrappedMentions`-painted (styled SGR substrings) and padded by `len(stripANSI)`; Ghostty's true-color SGR (`38;2;R;G;B`) width counting differed from rune counts, so `│` landed mid-row and the canvas fill behind the short row showed as a blob.
- `buildChatRows` subtracted the `[copy]` chip width from `contentWidth` for **every** row, so all rows wrapped 7 columns early.

## Fix

- `apps/local-runner/internal/tui/app/app.go` — boxed user messages take a dedicated branch in `buildChatRows`: `wrapText(msg.Content, width-3)` (box inner text width: left border + right border + leading space; no chip subtraction, no styled mention painting), `trimEmptyEdges`, 4-line clamp with `....` (kept), `[copy]` only on the last row. Non-user messages keep the shared markdown path.
- `apps/local-runner/internal/tui/app/chat_box.go` — `strokeChatRows` inner rows are plain: `base := " " + stripANSI(r.Text)` padded with plain runes to `innerW`; `[copy]` chip padded flush before the right border on the last row. Right border is exactly `innerW` for every row.
- Styled mention/file highlighting inside the You box is dropped (CA-558 highlights in the composer/`@` suggestions are untouched).

## Tests

- Updated `ca558_mention_highlight_test.go` — `TestRenderMessages_UserPromptShowsMentionText` asserts the plain mention text still appears in the box.
- New `apps/local-runner/internal/tui/app/you_box_lines_test.go` — prompt `[Change Contract]`/`feature: …`/`intent: … tra ve error khi b > a`/`symbols: Subtract` at 197/160 (F2 on) + 80 (F2 off) × claude/codex/grok: every original line present on its own box row, no `tra│` mid-word break, `[copy]` shares the last content row and sits before the right border; narrow 120 no-sidebar case: chip rides the wrapped content row.
- `go test ./internal/tui/app -count=1` 14s green, `go vet ./internal/tui/...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: render You box prompt lines plainly line by line at full inner width so borders and copy chip stay flush
# --->8---