# CA-684 — TUI quota chips stop fabricating ":0%" on informational usage lines

## Problem

Operator report during the CP-57 manual test guide run (2026-08-29): the TUI
session sidebar rendered opencode stats with a bogus percent suffix on every
row — "stats: 86 sessions · 15 days:0% cost: $137.86 total · $9.19/day:0%
tokens: 12.4M avg/session · 8.8K median:0% token limit: N/A …:0%".

Cause: `formatQuotaChip` (and the `collapsedQuotaChip` fallback) format EVERY
`UsageDetailLines` entry as a quota meter (`label:pct%`). CA-682's opencode
stats rows are informational text (`RemainingPercent=0`, no reset), so each got
a fabricated `:0%`. The stats content itself was correct; only the suffix was
wrong. (Opencode zen genuinely exposes no token limit — the "Limit: N/A" line
is by design; the `:0%` suffixes were the bug.)

## Fix (provider-agnostic)

- `formatQuotaChip`: a line with `pct <= 0` AND no reset renders as its bare
  label (informational), never `:0%`. Real meters keep their percent; a true
  0%-remaining meter WITH a reset still shows `:0% · resets …`.
- `collapsedQuotaChip` first-detail fallback: same guard.

## Tests

- `ca684_usage_stats_no_fake_percent_test.go` — bare informational render, no
  `:0%` on the reported fixture, meters unchanged, 0%-with-reset keeps percent
  + reset, `formatAccountLimits` + `collapsedQuotaChip` opencode fixtures keep
  stats content without the suffix. Old grok/gemini quota tests untouched and
  green.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: TUI quota chips render informational opencode stats lines without a fabricated 0 percent suffix
# --->8---
