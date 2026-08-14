---
id: CA-447
feature_key: ai-providers
title: Show Grok remaining quota from CLI billing?format=credits
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: ai-providers
prior_ca: CA-446-tui-skill-picker-statusline
will_not_undo: Task-216 team-credit /billing mapping; Grok runner ACP session/new
```

## Summary

Grok CLI 1.0.3 loads remaining usage with the cached `auth.json` bearer token and `GET /v1/billing?format=credits` (not a disk quota cache, not plain `/billing`). FlowPilot called `/billing`, got `monthlyLimit=0`, and dropped the meter, so Desktop and TUI showed no remaining. Both now map the credits-format weekly remaining.

## Changes

- Local-runner Grok metadata fetches `/billing?format=credits` first, then falls back to `/billing`.
- `creditUsagePercent` is used-percent (omitted = 0% used → 100% remaining). Weekly lines also fill `remaining_7d_percent` for TUI statusline.
- Admin-web `_account-metadata.ts` twin matches.
- TUI statusline can render `usage_detail_lines` when 5h/7d windows are empty.

## Provider impact

Grok account-quota display only. Claude/Codex/Gemini quota fetch paths unchanged. Grok ACP runner unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-216
change_type: bugfix
summary: Fetch Grok remaining quota via billing?format=credits like the CLI
# --->8---
