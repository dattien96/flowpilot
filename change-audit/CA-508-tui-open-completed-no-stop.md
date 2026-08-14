---
id: CA-508
feature_key: cli-tui
title: Open completed flow must not arm status [stop]
date: 2026-08-14
status: COMPLETE
---

## Change

`/open` of a finished workflow still starts orch SSE (late events / steps), but
`turnIsActive` treated any flow+`orchStream` as live work → status showed
`[stop]` (e.g. run-98153).

- Terminal run statuses skip the flow-orch arm of `[stop]`.
- `ChatOpenedMsg` prefers snapshot/history `Status` onto the handle when resume
  leaves it empty.
- Active agents / ConnRunning / non-terminal + orch still arm stop.

## Provider impact

Case 1 (agnostic). TUI only.

## Tests

New `tui_open_completed_no_stop_test.go`. Old `TestTurnIsActive_FlowOrchStillActive`
(empty status + orch) stays green (unknown status still arms).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Do not show [stop] after opening a completed flow run (orch listener only)
# --->8---
