# CA-541 - composer freeze: bracketed paste routed typing through the clipboard

## Problem

The TUI composer could "freeze": the `chat`/`next` prefix stayed on screen and
F2/F4 still worked, but typed characters never appeared. Root cause: Bubble Tea
delivers Windows Terminal's Ctrl+V as bracketed paste (`KeyRunes` with
`Paste: true`), and `handleKey` routed EVERY such keystroke through
`cmdClipboardPasteWithFallback`, which reads the system clipboard (native
`clipboard.Read` first, then a `powershell` `[System.Windows.Forms.Clipboard]::GetText()`
subprocess). A clipboard locked by another process blocks that read forever, so
the runes were consumed by the hung command and never inserted. Because F2/F4
are handled in earlier `switch` cases, they kept working while chat input looked
frozen. Plain-typing keys normally arrive `Paste: false`, but any paste that
stays in bracketed mode (or is stuck mid-sequence) tripped the path for every
following keystroke.

## Fix (TUI-only)

- `handleKey` `KeyRunes`/`Paste` branch (`app.go`): the bracketed-paste runes
  already carry the pasted text, so plain paste text now inserts directly with
  `insertInputAtCursor` — no clipboard read on the typing hot path. The
  clipboard is consulted only in the two cases where the runes cannot be enough:
  an empty/whitespace paste (image-only clipboard) and a pasted image file path.
- Added `cmdAttachImagePath` (`clipboard_paste.go`): reads the image file from
  disk via `client.ValidateAttachments` with no clipboard round-trip.
- Added `runClipboardPS` + `clipboardPSTimeout` (3s): every Windows PowerShell
  clipboard read (`GetText`, `GetImage`, `GetFileDropList`) is now bounded by
  `exec.CommandContext`, so even explicit paste degrades to an error instead of
  hanging the TUI forever.
- Explicit Ctrl+V / Alt+V / `ctrl+shift+v` paste contract is unchanged.

## Files

- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/clipboard_paste.go`
- `apps/local-runner/internal/tui/app/clipboard_image_paste_keys_test.go`
  (updated `TestBracketedPaste_*` to the new insert-directly behavior)
- `apps/local-runner/internal/tui/app/ca541_composer_paste_freeze_test.go` (new)

## Tests

Additive: `ca541_composer_paste_freeze_test.go` covers plain-text paste inserts
directly (3 providers), stuck single-rune paste inserts, empty paste still reads
clipboard, image-path paste attaches from disk, non-image path inserts as text,
explicit Ctrl+V/Alt+V still dispatch, and `imagePathFromClipboardText` guards.
The pre-existing `TestBracketedPaste_DispatchesClipboardPasteCmd` was updated to
`TestBracketedPaste_PlainTextInsertsDirectly` (operator-approved) because it
locked the frozen behavior.

## Verification

- `go build ./...`, `go vet ./internal/tui/app/`, gofmt clean (CRLF repo).
- `go test ./internal/tui/app/ -count=1` green except the two pre-existing
  environmental `TestCmdFocusAgent_*` network failures (confirmed identical on
  the stashed pre-change tree).

## Change Ledger

```json
{
  "flowpilot:change-ledger": {
    "source_doc_id": "CA-541",
    "change_type": "bugfix",
    "feature_key": "cli-tui",
    "impacted_areas": ["tui-composer-paste", "tui-clipboard"],
    "upstream_docs": ["SS-07", "CP-05"],
    "status": "verified"
  }
}
```
