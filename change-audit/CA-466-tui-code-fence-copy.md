---
id: CA-466
feature_key: cli-tui
title: Per-fence [copy] chip copies code only
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-465-tui-prompt-hug-codebox
will_not_undo: CA-465 You hug box; CA-464 goldmark cache; CA-460 answer [copy]
```

## Change

- Each fenced code box gets its own `[copy]` on the title row. Click copies the fence body (raw source, not the truncated display and not the rest of the reply).
- The existing answer `[copy]` still copies the full assistant/user message.
- TUI chrome only (`internal/tui/app`).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-289
change_type: feature
summary: Add a per-code-block [copy] chip while keeping full-reply copy
# --->8---
