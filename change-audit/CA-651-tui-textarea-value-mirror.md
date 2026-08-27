# CA-651: TUI composer adopts bubbles/textarea as value-only mirror (Task-308 Phase 1)

## What

Phase 1 of Task-308: embed `github.com/charmbracelet/bubbles/textarea` into the
TUI composer as a **value-only mirror** of the live composer
(`m.inputValue`/`m.inputCursor` stay the single source of truth).

- `model.go`: `textarea textarea.Model` + `textareaReady bool` (set in `New()`).
- `chat_textarea.go` (new): `newChatTextArea` (Prompt `┃ `, no line numbers,
  `KeyMap.InsertNewline`/`KeyMap.Paste` disabled so `handleKey` owns
  Enter/Shift+Enter and Alt+V); `syncTextareaValue()` pushes `inputValue` only
  when `authPhase == AuthNone` (no password leak), guarded by a normalized-form
  compare to avoid churn; `mirrorReady()`/`isTextareaReady()` helpers.
- Fidelity: `normalizeComposerForMirror` + `expandMirrorTabs` push the exact
  form the bubbles sanitizer produces (`\r`/`\n` -> `\n`, `\t` -> 4 spaces,
  C0 dropped). The live composer is never rewritten. Known Phase-2 constraint:
  `Value()` trims one trailing newline (input ending in >=2 newlines).
- `chat_input_cursor.go`: `clearInputValue` calls `textarea.Reset()`; insert/
  delete paths sync the mirror once. `handleKey` nav keys unchanged (Left/
  Right/Home/End/BS/Del/Up/Down/SShift+Enter all still use the legacy helpers).
- `chat_paste.go` + both Windows raw-paste reject branches now call
  `syncTextareaValue()` after mutating `inputValue` (mirror invariant).
- `go.mod`: `bubbles v0.21.0` promoted to direct require.
- No `unsafe`/`reflect` on bubbles private fields (Round-1 review finding
  removed); no typing/nav delegation to `textarea.Update` (Phase 2).

## Why

Round-1 review found the first attempt used `unsafe`+`reflect` to poke bubbles
private `row`/`col`/`value` (panic risk on library bump) and claimed more
delegation than it delivered. This Phase 1 keeps behavior identical to HEAD
(verified by full diff against pre-change code) while establishing the mirror
a future Phase 2 can adopt for `textarea.View()`.

## Tests

- `task308_textarea_test.go` (new, additive — CP-56 D-8): UTF-8/multi-rune
  mirror fidelity, caret nav via legacy helpers, fast typing, paste token
  collapse/expand, slash suggestions, Ctrl+C clear/quit, cross-provider parity,
  Shift+Enter newline, auth no-leak, tab/CRLF normalization, reject-revert sync,
  collapse-burst sync.
- Legacy matrix untouched. `go vet ./internal/tui/app` clean;
  `go test ./internal/tui/...` 6/6 packages pass.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-308
change_type: refactor
summary: embed bubbles/textarea as value-only mirror in TUI composer (Phase 1), keep inputValue live, sync guarded
# --->8---