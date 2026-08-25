# CA-630 — TUI Universal Alt-V / NUL Key Fix & Burst Reset

**Date**: 2026-08-25  
**Author**: Antigravity  
**Ticket**: CA-630

---

## Problem

After pressing Alt-V to paste in Windows Terminal (ConPTY), the chat input box became completely unresponsive — no characters could be typed. Root causes identified from `tui.log`:

1. **NUL key injection**: ConPTY/Windows Terminal emits `KeyRunes = ['\x00']` (alt+\x00) on Alt modifier press. The old `handleKey` had no filter for these, so `\x00` was inserted directly into `inputValue`, corrupting the paste token matching.
2. **`rejectArmed` swallow trap**: Ctrl+V flooded `rejectArmed = true`, but `ClipboardPasteMsg` (Alt+V result) never disarmed it — so all fast typing after Alt+V was swallowed.
3. **OS-branched flood reject logic** (`if m.rejectWindowsRawPaste`) scattered across 4 places made universal behaviour difficult to reason about.

---

## Fix

### [`app.go`](file:///C:/working/flowpilot/apps/local-runner/internal/tui/app/app.go)

1. **`ClipboardPasteMsg` handler** — `m.resetPasteBurst()` called immediately so clipboard paste always disarms any armed burst.
2. **`pasteBurstSettleMsg`** — unified: if `rejectWindowsRawPaste` + `rejectArmed`, revert input to `pasteBurst.start` on settle; otherwise collapse to token normally. Shows Alt+V hint toast once per flood after settle.
3. **`handleBurstNewline` path** — `rejectWindowsRawPaste` reject checks restored as-is (needed for existing CA-612 tests).
4. **`KeyCtrlV`** — supports both `rejectWindowsRawPaste` (arm + hint + settle loop) and plain `runtime.GOOS == "windows"` (hint-only, for production without explicit reject flag).
5. **`KeySpace/KeyRunes`** — NUL filter added at entry (non-bracketed paste only): drops `alt+\x00`, `\x00`, and all-NUL rune arrays universally. Burst reject logic preserved beneath filter. NUL stripped from rune string before insert.
6. **`renderInputLine`** — `pasteBurst.active` display no longer gated by `!m.rejectWindowsRawPaste`; `[Pasting…]` placeholder shown universally while burst active.

### [`chat_input_cursor.go`](file:///C:/working/flowpilot/apps/local-runner/internal/tui/app/chat_input_cursor.go)

- `insertInputAtCursor(s)` — `strings.ReplaceAll(s, "\x00", "")` as defensive NUL strip before any insert.

### [`tui_windows_altv_nul_key_test.go`](file:///C:/working/flowpilot/apps/local-runner/internal/tui/app/tui_windows_altv_nul_key_test.go) _(new file)_

Additive test suite — 4 tests × 3 providers (claude/codex/grok) = 12 sub-tests:
- `TestCA630_NulKeyNoiseFiltered_ClaudeCodexGrok` — alt+\x00 and lone \x00 never enter inputValue
- `TestCA630_AltVPasteAndImmediateTyping_ClaudeCodexGrok` — Alt+V collapses, subsequent typing immediately appended
- `TestCA630_PasteTokenSingleBackspaceDelete_ClaudeCodexGrok` — 1 Backspace clears [Pasted N chars] token even after NUL noise
- `TestCA630_RawStreamPasteCollapsesOnSettle_ClaudeCodexGrok` — raw char stream collapses to token on 150ms settle

---

## Test Results

```
ok  flowpilot-runner/internal/tui/app       13.770s
ok  flowpilot-runner/internal/tui/client    37.944s
ok  flowpilot-runner/internal/tui/config    (cached)
```

All 12 new CA-630 sub-tests pass. All pre-existing tests remain green.

---

## Architecture Note

No `runtime.GOOS == "windows"` branches in paste flood or burst tracking. OS check only in `KeyCtrlV` (show hint on Windows) and in `Run()` entrypoint (set `rejectWindowsRawPaste = true` for production). Tests control behaviour via `m.rejectWindowsRawPaste` field directly, keeping all burst+collapse logic testable cross-platform.
