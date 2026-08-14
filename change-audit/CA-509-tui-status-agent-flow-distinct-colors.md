---
id: CA-509
feature_key: cli-tui
title: Distinct status colors for agent and flow values
date: 2026-08-14
status: COMPLETE
---

## Change

Status line values no longer share accent (`styleStatusHi` / `stylePromptFocus`)
with model, reasoning, YOLO, skills:

- `agent:<name>` value → teal `styleStatusAgent` (`#2dd4bf`)
- Flow name + active step on mode line → pink `styleStatusFlow` (`#f472b6`)
- Mode chip `[flow]` stays dim; only the flow values are colored

## Provider impact

Case 1 (agnostic). TUI chrome only.

## Tests

New `tui_status_agent_flow_color_test.go`; update co-session agent color asserts.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: task
summary: Status agent and flow values use dedicated teal/pink colors not shared with model
# --->8---
