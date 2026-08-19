# CA-560: composer paste-summary — long multi-line paste collapses to a token, never auto-submits

## Fix

Pasting a long multi-line prompt into the TUI chat composer no longer looks
"cut" and never auto-submits partial lines.

**Paste summary (placeholder).** A paste whose line breaks would overflow the
6-line composer (>= 6 newlines) collapses to a `[Pasted N lines · C chars]`
token in the input box instead of inserting the raw text (opencode-style). The
full text is stored in `pasteSegments` and expanded on submit, so the timeline
and prompt history receive the real prompt. Single-line pastes (paths, URLs,
code) always insert verbatim — only multi-line blocks look cut. Applied to both
bracketed paste (`KeyRunes+Paste`, the Windows Terminal Ctrl+V path) and the
explicit clipboard fallback (`ClipboardPasteMsg.Text`).

**Raw-paste burst guard.** When the terminal delivers the paste as a flood of
individual key events (no bracketed paste — conhost / Ctrl+Shift+V / right-click
paste), every line break arrives as a plain Enter and flowpilot used to submit
each line separately ("auto send"). `pasteBurst` tracks the rapid rune flood
(runes closer than 25ms apart — impossible for human typing): while the burst is
active, Enter becomes a newline instead of a submit, and once the flood settles
the tracked region collapses to the same paste token. Slash-command inputs
(starting with `/`) are never burst-guarded, so fast programmatic/typed keys
still run their command on Enter.

## Where

`apps/local-runner/internal/tui/app/chat_paste.go` (new): `pasteSummaryToken`,
`needsPasteSummary`, `pasteSegment`, `pasteBurst`, `noteBurstRune`,
`handleBurstNewline`, `collapsePasteBurst`, `insertPasteSummary`,
`expandPasteTokens`, injectable clock `pasteNow`.

`app.go`: `handleKey` top — burst collapse + newline swallow (Enter during an
active raw-paste flood); bracketed-paste branch and `ClipboardPasteMsg` collapse
long multi-line pastes; `KeySpace/KeyRunes` tracks burst runes; submit path
expands tokens (`strings.TrimSpace(m.expandPasteTokens(m.inputValue))`).

`model.go`: `pasteSegments []pasteSegment` and `pasteBurst pasteBurst` fields.

`chat_input_cursor.go`: `clearInputValue` resets segments + burst.

## Tests

`chat_paste_summary_test.go` (new): token format, collapse thresholds, bracketed
and clipboard long-paste collapse, short paste verbatim (CA-541 contract kept),
Enter-sends-full-expanded-text, segment reset, two-collapse ordering, raw-paste
Enters→newlines, burst collapse to token, short-burst verbatim, slow typing
submit, Enter-after-settle collapse+submit. All `ca541_composer_paste_freeze_test.go`
and `TestA8*` slash tests stay green. Pre-existing `TestCmdFocusAgent_*` network
failures are unrelated.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: long multi-line pastes collapse to a '[Pasted N lines · C chars]' token in the TUI composer and expand on submit; raw-paste bursts convert Enter to newline instead of auto-submitting each line, with slash commands never burst-guarded
# --->8---