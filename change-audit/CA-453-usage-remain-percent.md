---
id: CA-453
feature_key: cli-tui
title: Context remaining % and input tokens on TUI statusline and Desktop usage line
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-452-grok-midchat-model
will_not_undo: CA-447 Grok billing credits; CA-448 remaining chip first; CA-450 reset date
```

## Grok CLI `/usage` vs FlowPilot

Grok CLI `/usage` shows weekly credits remaining, context-window remaining %, user/input tokens, and a cost-like line. FlowPilot already had weekly remaining on the TUI (`7d:N%`). Context was `ctx used/window (left)` with no remain % and no input tokens. Desktop `usageSummaryLine` had used/left and in/out, no remain %, no credit chip.

USD/cost is **not** shown: Grok ACP `_meta` has no dollar field. Inventing a price would be wrong. Weekly credits remain the `/usage` credit line.

## Change

- TUI `formatContextLimits`: `ctx 91% remain · 12.0k/128.0k (116.0k left) in:500`. Old test still matches `left` + `12.0k`.
- Desktop usage line: credit chip from the active account (`7d N%`), context remain %, existing used/left and in/out.

Provider-agnostic formatters over the shared `TokenUsageSnapshot` (Claude/Codex/Grok).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: Show context remaining percent and input tokens on TUI statusline and Desktop usage line
# --->8---
