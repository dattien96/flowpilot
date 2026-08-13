---
id: CA-464
feature_key: cli-tui
title: Codex-style goldmark assistant markdown with per-message cache
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-463-tui-raw-markdown-box
will_not_undo: CA-463 markdown-labeled box; CA-460 copy glued to answer; chatRows scroll cache
```

## Change

- Assistant `FormatHint==""` turns parse GFM via goldmark and walk to Lip Gloss lines (headings, lists, fences, tables, inline). No Glamour/chroma.
- `renderMarkdown` caches by `(width, ascii, content hash)` (96 entries). Scroll/`View()` reuse `chatRows` + this cache; parse counter tests lock that.
- Tables emit padded pipes when they fit, otherwise wrap cells. Fences are monospace with a 200-line / 32KiB head+tail bound. `[copy]` still uses raw `msg.Content`.
- TUI chrome only (`internal/tui/app`). Task-289.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-289
change_type: feature
summary: Render TUI assistant markdown with goldmark and a per-message cache like Codex
# --->8---
