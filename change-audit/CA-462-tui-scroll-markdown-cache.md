---
id: CA-462
feature_key: cli-tui
title: Cache Glamour markdown and chat rows so scroll does not re-parse history
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-461-tui-glamour-full-document
will_not_undo: CA-461 single Glamour/Goldmark pass; CA-460 copy glued to answer
```

## Change

- `renderMarkdown` caches Goldmark/Glamour output by width+hash (96 entries). Scroll/cursor ticks reuse history instead of re-highlighting every assistant message.
- `chatRows` is cached on the model by transcript signature (width, ascii, messages). Viewport offset is not part of the key, so wheel-scroll only slices the cached rows.
- Glamour Tokyo Night style is built once.

TUI chrome only. Root cause: View() + mouse hit-testing called `chatRows()` → Glamour on the full transcript every frame.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Cache Glamour markdown and chat rows so scrolling does not re-parse the transcript
# --->8---
