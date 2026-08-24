# CA-624: Alt+V paste token not deletable with Backspace/Esc

## What

Alt+V paste of a long/multi-line block collapsed to `[Pasted N chars]` (`insertPasteSummary`) but Backspace deleted only one rune (`]`), leaving the token requiring many presses. User reported Backspace and Esc both failed after Alt+V paste (Alt+V text `PanlObjectCatalog` seen in `tui.log` `ClipboardPasteMsg`, followed by a single Backspace).

## Why

- `deleteInputBeforeCursor` always deleted exactly one rune `cur-1`, unaware of collapsed paste tokens. A token like `[Pasted 20 chars]` (20 chars) needed 20 Backspaces.
- `Esc` with active `mouseSel` only cleared selection (contract `TestEscape_ClearsSelectionBeforePrompt`: first Esc clears selection, second clears prompt). Users with a drag-armed selection perceived Esc as doing nothing.
- `ClipboardPasteMsg` body was logged via `Update %T %v` (default branch), dumping full paste text into `tui.log` and stalling the event loop (same class as CA-621).

Typing short text (no token) was not affected; only collapsed pastes.

## Fix

- **tui/app/chat_input_cursor.go** `deleteInputBeforeCursor`: if caret is right after a token matching any `pasteSegments[].token`, remove the whole token and that segment (and `resetPasteBurst`), otherwise single-rune delete. Sticky-end stays sticky.
- **tui/app/chat_input_cursor.go** new `deleteInputAfterCursor`: forward Delete mirrors the same token check when caret is before a token.
- **tui/app/app.go** `handleKey` `KeyBackspace`/`KeyDelete`: add `KeyDelete` case calling `deleteInputAfterCursor`. `Esc` keeps legacy two-step contract (first clears selection, second clears input) — no change to `TestEscape_ClearsSelectionBeforePrompt`; token is cleared on second Esc or by one Backspace.
- **tui/app/app.go** `Update` logging: `ClipboardPasteMsg` now logs `textLen=` + flags, not body (CA-621 follow-up). Keeps `View slow` throttle (CA-621) and `sessionLoading` UX.

Keeps CA-619/620/622 blocked bar, CA-621 log summarize, CA-623 freeze stamp, IME `resetPasteBurst` after Backspace (CA-611), `clearInputValue` segment reset.

## Tests (additive, no old edit)

- `tui/app/ca624_paste_token_delete_test.go` (provider-agnostic: input editing never branches on `ProviderKey`; table `claude/codex/grok` guards future):
  - `TestCA624_BackspaceDeletesPastedToken` — `ClipboardPasteMsg` long paste → token, one Backspace → `""` + segments empty (table 3 providers).
  - `TestCA624_EscClearsPastedToken` — token then Esc (and `KeyEsc` alias) → cleared.
  - `TestCA624_TokenWithTrailingText_BackspaceOnlyRemovesToken` — token + ` hello`, caret after token → Backspace leaves ` hello`.
  - `TestCA624_TypedShort_NotAffected` — `abc` Backspace → `ab` (no regress).
  - `TestCA624_DeleteForwardRemovesToken` — `KeyDelete` before token → token removed.
  - `TestCA624_EscWithSelectionAndInputClearsBoth` — first Esc clears selection, second clears token (locks `TestEscape_ClearsSelectionBeforePrompt`).

Old: `go vet ./internal/tui/app ./internal/runner` pass; `go test ./internal/tui/app` 24s pass (including `TestEscape_ClearsSelectionBeforePrompt`, `TestBackspaceAtEnd_KeepsStickyEnd`, `TestPasteSummary_*`); `go test ./internal/runner -run TestCA623` pass. ClampChecked #356 is F1 payload, out of scope.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Alt+V paste token deletes as whole with Backspace/Delete and Esc — token-aware delete avoids many-presses (CA-624)
# --->8---
