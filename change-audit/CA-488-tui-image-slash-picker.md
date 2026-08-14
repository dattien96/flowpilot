---
id: CA-488
feature_key: cli-tui
title: /image Tab picker + panel open button
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-487-tui-attach-panel-detach
will_not_undo: panel [x] detach; chip open panel; max 6
```

## Change

1. **Slash picker** (like `/flow` / `/provider`): `/image ` → Tab options
   `open|paste|list|clear|rm`. Choosing `open`/`rm` leaves a trailing space so
   the next picker lists pending images (Enter runs `/image open N`).
2. **Panel**: each row has **`[open]`** (preview) and **`[x]`** (remove).
   Name/meta click also opens. Two ways to preview: slash picker + panel.

## Tests

`image_suggestions_test.go`; attach panel hit tests updated for [open]/[x].

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI /image Tab picker for open and panel [open] next to [x]
# --->8---
