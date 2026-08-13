---
id: CA-469
feature_key: cli-tui
title: Six-row collapsible TUI status bar
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-468-tui-narrow-input-wrap
will_not_undo: CA-468 last-column wrap; CA-467 flow account/stop-all
```

## Change

Status chrome is now a stacked bar:

- Line 0 (always): fold chip `F4`, `[stop]` when live, ready/running/…, SIGN-IN, agents
- Line 1: `[chat]` (dim) or highlighted `[flow] name` / `[step]`
- Line 2: model · reasoning · YOLO (auto-on still labeled in flow). No provider next to the model.
- Line 3: current account · quota · reset time (provider key only here, next to the account)
- Line 4: context / token usage
- Line 5: project + git branch

F4 or click line 0 (except `[stop]`) hides lines 1–5. F3 skills chip stays on the model row.

TUI chrome only.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: Restack TUI status into a 6-row bar with F4/click collapse keeping the status row visible
# --->8---
