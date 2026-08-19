# CA-560: composer paste-summary — paste collapses to one "[Pasted N chars]" token, never auto-submits

## Fix

Pasting any text block into the TUI chat composer now collapses to a single
generic line `[Pasted N chars]` (opencode-style) instead of inserting the raw
text, so a long multi-line prompt never looks "cut" and never auto-submits
partial lines. The full text is stored in `pasteSegments` and expanded on
submit, so the timeline and prompt history receive the real prompt. Manual
typing is unlimited — the composer no longer clamps to 6 lines, so a typed
multi-line prompt renders fully and the caret can never be lost.

**Paste summary.** Any paste that is multi-line OR at least 8 runes collapses to
`[Pasted N chars]`; tiny snippets and single keystrokes (stuck-paste typing)
still insert verbatim. Applied to bracketed paste (`KeyRunes+Paste`, the Windows
Terminal Ctrl+V path) and the explicit clipboard fallback (`ClipboardPasteMsg.Text`).

**Raw-paste burst guard.** When the terminal delivers the paste as a flood of
individual key events (no bracketed paste — conhost / Ctrl+Shift+V / right-click
paste), every line break arrives as a plain Enter and flowpilot used to submit
each line separately ("auto send"). `pasteBurst` tracks the rapid rune flood
(runes closer than 25ms apart — impossible for human typing): while the burst is
active, Enter becomes a newline instead of a submit, and once the flood settles
the tracked region collapses to the same token. A `pasteBurstSettleMsg` tick
(150ms) collapses an idle burst automatically — no keystroke needed. Slash
commands (starting with `/`) are never burst-guarded, so fast programmatic/typed
keys still run their command on Enter.

**No composer clamp.** The old 6-line `maxVis` clamp in `renderInputLine` and
`tryPlaceInputCursor` is removed. Clamping computed the shown window's line
offsets from 0 instead of the real input offset, so once input exceeded 6 lines
the caret was drawn at the wrong row and effectively vanished (arrow keys and
mouse clicks stopped "finding" it). With the clamp gone and pastes collapsed to
one token, the caret always tracks correctly.

## Where

`apps/local-runner/internal/tui/app/chat_paste.go` (new): `pasteSummaryToken`,
`needsPasteSummary` (multi-line or >= `pasteCollapseMinRunes`), `pasteSegment`,
`pasteBurst`, `noteBurstRune`, `handleBurstNewline`, `collapsePasteBurst`,
`insertPasteSummary`, `expandPasteTokens`, `pasteBurstSettleMsg` +
`cmdPasteBurstSettle`, injectable clock `pasteNow`.

`app.go`: `handleKey` top — burst collapse + newline swallow + settle-tick arm;
bracketed-paste branch and `ClipboardPasteMsg` collapse paste blocks; `Update`
handles `pasteBurstSettleMsg`; `KeySpace/KeyRunes` tracks burst runes and re-arms
the settle tick; submit path expands tokens; `renderInputLine` no longer clamps.

`model.go`: `pasteSegments []pasteSegment` and `pasteBurst pasteBurst` fields.

`chat_input_cursor.go`: `clearInputValue` resets segments + burst;
`tryPlaceInputCursor` no longer clamps.

## Tests

`chat_paste_summary_test.go` (new): token format, collapse thresholds, bracketed
and clipboard collapse (multi-line and long single-line), tiny snippets verbatim,
Enter-sends-full-expanded-text, segment reset, two-collapse ordering, raw-paste
Enters→newlines, burst collapse to token, settle-tick auto-collapse, short
single-line burst verbatim, slow typing submit, Enter-after-settle
collapse+submit, no-composer-clamp render.

Updated to the new collapse behavior (approved): `ca541_composer_paste_freeze_test.go`
`TestBracketedPaste_PlainTextNeverReadsClipboard` (multi-line → token) and
`TestBracketedPaste_NonImagePathInsertsAsText` (path → token);
`clipboard_image_paste_keys_test.go` `TestBracketedPaste_PlainTextInsertsDirectly`
("some text" → token). `TestBracketedPaste_SingleRuneStuckPasteInserts`,
`TestBracketedPaste_EmptyRoutesClipboard`, `TestBracketedPaste_ImagePathAttachesFromDisk`,
`TestClipboardPasteMsg_InsertsTextAtCursor` (" world" stays verbatim) and the
`TestA8*` slash tests stay green. Pre-existing `TestCmdFocusAgent_*` network
failures are unrelated.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: any paste block collapses to a one-line '[Pasted N chars]' token in the TUI composer and expands on submit; raw-paste bursts convert Enter to newline and auto-collapse via settle tick; composer no longer clamps to 6 lines so the caret is never lost, and manual typing is unlimited
# --->8---