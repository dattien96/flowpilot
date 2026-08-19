# CA-562: scrub NUL/C0 control bytes from pasted text — a "\u0000"-prefixed prompt copied as empty and refused to expand

## Bug

run-117747's turns log showed a prompt starting with a NUL byte:

```
{"kind":"prompt","turn_id":"turn-117754","prompt":"\u0000Prompt:\n[Change Contract]\nfeature: calc-core\nintent: ..."}
```

The user pasted a block that carried a leading `\u0000` (copied from a tool/terminal).
Symptoms: the sent prompt could not be copied via the `[copy]` chip (empty everywhere —
Notepad, web, editors) and the 4-line-clamped bubble could not be trusted (leading line
renders invisible). A normal typed message copied fine, which is why the failure looked
tied to "pasted text".

Root cause: the message `Content` kept the raw NUL byte. `writeClipboardText` encodes the
text to UTF-16 for the Windows clipboard, the first code unit `0x0000` is read back as the
string terminator, so the OS stores an EMPTY string even though the app "copied" the full
prompt. Reproduced in a test: `clipboard after copy len=0 bytes=""` while the model held the
full `\x00Prompt:...` content.

## Fix

Two layers:

1. **Paste sanitization** — new `sanitizePasteText` strips C0 control bytes (NUL and
   `< 0x20`, plus DEL `0x7f`) while preserving `\n \r \t`. Applied at every paste entry:
   bracketed paste (`app.go` `KeyRunes+Paste`), `ClipboardPasteMsg.Text`, and raw-burst
   collapse (`collapsePasteBurst` — the tracked region is rewritten to the scrubbed text,
   and sub-threshold bursts still drop the junk bytes). The composer, stored message, token
   char count, and prompt history therefore never carry NUL.
2. **Clipboard safety net** — `writeClipboardText` also sanitizes before writing, so any
   existing/replayed message with a stray control byte copies its real content instead of
   silently truncating to an empty clipboard.

## Where

`apps/local-runner/internal/tui/app/chat_paste.go` — `sanitizePasteText`, `collapsePasteBurst`
(guard now matches raw, then rewrites to scrubbed text).
`apps/local-runner/internal/tui/app/app.go` — bracketed-paste and `ClipboardPasteMsg` call
`sanitizePasteText` before summary/verbatim insertion.
`apps/local-runner/internal/tui/app/clipboard_paste.go` — `writeClipboardText` sanitizes.

## Tests

New (additive): `chat_paste_summary_test.go`
`TestPaste_NulBytesScrubbedFromPrompt` — NUL-prefixed multi-line paste collapses to a clean
token, sends a message whose `Content` starts with the real text and contains no NUL.
`TestSanitizePasteText_StripsControlBytes` — C0/DEL stripped, `\n \r \t` preserved. The full
`internal/tui/app` suite stays green.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CA-560
change_type: bugfix
summary: sanitizePasteText strips NUL and C0 control bytes from pasted text (and writeClipboardText before writing) so a '\u0000'-prefixed prompt is copied and expanded normally instead of producing an empty clipboard (run-117747)
# --->8---