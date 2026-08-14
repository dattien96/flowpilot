---
id: CA-507
feature_key: cli-tui
title: agent:name status highlight; F2 drop Viewing; open/back color
date: 2026-08-14
status: COMPLETE
---

## Change

- Status: `view:xxx` → `agent:xxx` with dim prefix + accent-highlighted name.
- F2 steps: remove duplicate `Viewing: … [back]` header (focused step row already
  highlights + shows `[back]`).
- `[open]` / `[back]` use `styleStepAgentAction` (ask/purple), distinct from step
  highlight accent and running warn.

## Provider impact

Case 1 (agnostic). TUI chrome only.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: task
summary: Status agent:name with name highlight; F2 step open/back purple chips only
# --->8---
