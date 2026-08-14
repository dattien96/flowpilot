---
id: CA-448
feature_key: cli-tui
title: Keep Grok remaining quota visible on TUI statusline
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-447-grok-billing-credits-remaining
will_not_undo: CA-447 billing?format=credits fetch; Desktop account meters
```

## Summary

Desktop showed Grok weekly remaining after CA-447, but TUI statusline truncated `7d:N%` away because it sat after YOLO/context and the width trim drops second-to-last chips first. Remaining is now the first status chip and also appears on the F2 session line.

## Provider impact

TUI chrome only. Claude/Codex/Grok adapters unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-216
change_type: bugfix
summary: Pin Grok remaining quota as first TUI status chip so it is not truncated
# --->8---
