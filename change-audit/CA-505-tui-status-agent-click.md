---
id: CA-505
feature_key: cli-tui
title: Click agents chip focuses agent without F4 fold
date: 2026-08-14
status: COMPLETE
---

## Change

Status-line `agents:` name clicks were swallowed by `status-details` (F4 fold).
`hitStatusAgentsChrome` now maps each agent token to `agent-open:<runId>` and
is checked **before** `hitStatusDetailsChrome`. Dispatch reuses existing
`cmdFocusAgent` / agent-back paths.

## Provider impact

Case 1 (agnostic). UI hit-test only.

## Tests

New `tui_agent_status_click_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Status-bar agent name clicks open that agent instead of collapsing F4 details
# --->8---
