---
id: CA-461
feature_key: cli-tui
title: Assistant markdown is one Glamour/Goldmark Render, not a char scanner
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-460-tui-grok-table-copy-input
will_not_undo: CA-460 copy glued to answer; rounded input stroke
```

## Change

- `renderMarkdown` calls Glamour once on the full source. Goldmark parses GFM (headings, tables, fences, lists). The homemade `parseMdBlocks` / per-character `styleInlineMarkdown` path is fallback only (narrow wrap / render error / leftover fences).
- Stopped splitting the document into hand-scanned chunks before Glamour.

TUI chrome only.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: refactor
summary: Render assistant markdown with a single Glamour/Goldmark pass instead of a character scanner
# --->8---
