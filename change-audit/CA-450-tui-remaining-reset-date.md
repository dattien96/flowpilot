---
id: CA-450
feature_key: cli-tui
title: Show remaining quota reset date on TUI like Desktop
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-449-tui-owned-runner-tree-kill
will_not_undo: CA-448 remaining-first status chip; CA-447 remaining_7d_reset_at API field
```

## Summary

Desktop accounts panel shows `{percent}% · resets {date}`. TUI only showed `7d:87%` because the client struct dropped `remaining_*_reset_at`.

## Changes

- Decode `remaining_5h_reset_at` / `remaining_7d_reset_at` on the TUI account summary.
- Format chips as `7d:87% · resets Aug 19, 06:46` (local time) when a reset timestamp is present.
- Missing reset keeps the old `5h:72% 7d:40%` contract.

## Provider impact

TUI chrome only. `formatAccountLimits` is provider-agnostic (Claude 5h + Grok 7d both render reset when the API sends it). Adapters unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-216
change_type: bugfix
summary: Show remaining quota reset date on TUI statusline like Desktop
# --->8---
