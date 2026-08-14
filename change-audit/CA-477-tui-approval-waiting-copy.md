---
id: CA-477
feature_key: cli-tui
title: TUI approval transcript waits instead of inviting click
date: 2026-08-14
status: COMPLETE
---

## Change

When a permission gate is shown, the input bar already has Approve / Deny.
The transcript line no longer says "click Approve or Deny" (that looked like
a second click target on the response). It is now a waiting line:

`⏳ [APPROVAL] <id> Waiting user…` (ASCII: `... [APPROVAL] <id> Waiting user...`).

Input-bar buttons and `/approve` `/deny` are unchanged. YOLO=on still does
not mount the card (CA-476).

## Provider impact

Provider-agnostic TUI chrome only.

## Tests

New `tui_approval_waiting_copy_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-291
change_type: bugfix
summary: TUI approval response line shows waiting copy; Approve/Deny stay in the input bar
# --->8---
