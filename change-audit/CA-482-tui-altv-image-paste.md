---
id: CA-482
feature_key: cli-tui
title: Fix Alt+V / bracketed-paste image attach routing
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-443-cli-tui-initial-implementation (clipboard images follow-up)
will_not_undo: CA-443 Windows GetImage/file-drop fallbacks; /image paste reserved subcommand; codex/claude vision gate
```

## Issue

Copy/paste image into the TUI looked broken on Windows:

1. **Ctrl+V** is often stolen by Windows Terminal (text-only bracketed paste).
2. Documented workaround **Alt+V** was a no-op for attach: Bubble Tea delivers
   Alt+letter as `KeyRunes` + `Alt=true` (`String() == "alt+v"`). `handleKey`
   handled `KeyRunes` first and inserted a literal `v`, returning before the
   post-switch `alt+v` chord handler.

Clipboard read helpers (`golang.design/x/clipboard` + PowerShell GetImage /
file-drop) still worked when invoked via `/image paste`.

## Change

- In `handleKey` `KeyRunes` path: dispatch `alt+v` / `ctrl+shift+v` **before**
  inserting runes.
- Bracketed paste (`KeyRunes` + `Paste=true`, typical WT Ctrl+V) routes through
  `cmdClipboardPasteWithFallback` so image/path attach is preferred; bracketed
  paste text is used when the native clipboard has no image/text.
- `cmdClipboardPaste` now delegates to `cmdClipboardPasteWithFallback("")`.

Provider gate unchanged: **codex / claude only** (Grok `Vision=false`).

## Tests

Additive `clipboard_image_paste_keys_test.go`:

- Alt+V KeyRunes → clipboard cmd, no `"v"` insert
- Bracketed paste → clipboard cmd (no immediate insert)
- Plain `v` still inserts
- Ctrl+V still dispatches paste
- Fallback text path when clipboard empty

## Blast radius (GitNexus MCP unavailable)

Symbols: `handleKey`, `cmdClipboardPaste`, new `cmdClipboardPasteWithFallback`.
Callers: Bubble Tea key loop, `/image paste`. Risk: low–medium (key routing);
mitigated by additive key tests + plain-v regression.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Fix TUI Alt+V and WT bracketed-paste so clipboard images attach instead of inserting v
# --->8---
