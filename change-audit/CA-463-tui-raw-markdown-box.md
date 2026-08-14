---
id: CA-463
feature_key: cli-tui
title: MVP raw markdown in a labeled box; drop Glamour/Goldmark and code boxes
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-462-tui-scroll-markdown-cache
will_not_undo: CA-460 copy glued to answer; rounded input; prompt You box; chat row cache
```

## Change

- Revert Glamour/Goldmark. Assistant turns show **raw markdown**, wrapped in a stroke box titled `markdown`.
- Removed fenced-code boxes. `renderMarkdown` is wrap-only.
- Dropped `github.com/charmbracelet/glamour` from the runner module.

TUI chrome only.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: refactor
summary: Show raw markdown in a markdown-labeled box and remove Glamour/Goldmark from the TUI
# --->8---
