---
id: CA-506
feature_key: cli-tui
title: F2 step [open]/[back]; status only view:main|name
date: 2026-08-14
status: COMPLETE
---

## Change

Sub-agent navigation moved off the status bar onto the F2 session/steps panel:

- **F2 steps**: child steps show `[open]`; while viewing a child, show `Viewing: <name> [back]`
  and step-row `[back]`. Mouse hit-test `hitSessionAgentChrome` only (panel lines).
- **Status line**: compact location only — `view:main` or `view:<sub-agent display name>`.
  No agent list, no `[back]` on status, no click-to-focus on status (supersedes CA-505).

## Provider impact

Case 1 (agnostic). TUI chrome only.

## Prior CA not undone

- CA-499 step open mapping, CA-503 hydrate, CA-504 resume child transcript remain.
- CA-505 status-bar agent click is intentionally replaced by this UX.

## Tests

- Rewrite co-session `tui_agent_status_click_test.go` for view-only status.
- Update `TestFormatAgentViewStatus_*`; extend `tui_step_agent_open_test.go`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: task
summary: Agent open/back on F2 steps panel; status line shows view:main or view:name only
# --->8---
