---
id: CA-511
feature_key: cli-tui
title: Flow quiet chat; step notices main-only; copy toast
date: 2026-08-14
status: COMPLETE
---

## Change

1. **Flow chrome** (`ModeFlow` / `ModeStep` / catalog workflow): do not inject
   `thinking…` assistant placeholders into the chat transcript. Status uses
   step/flow labels instead of thinking/turn-running when appropriate.
2. **Agent open**: remove `Viewing agent:` / `Child transcript:` system banners
   (status `agent:name` + F2 highlight are enough).
3. **Step progress** (`> N. [RUNNING] …`): only append to chat when viewing
   **main**; while focused on a sub-agent, update F2/status only.
4. **Copy feedback**: `Copied answer/selection/prompt` is a 1s flash toast
   above the status bar — not a chat timeline system message. Copy errors
   still use system error lines.

## Provider impact

Case 1 (agnostic). TUI chrome only.

## Tests

New `tui_flow_chrome_quiet_test.go`. Update `flow_mode_agents_stop_test` for
silent focus.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: task
summary: Flow mode quiet chat; step notices main-only; copied toast 1s not timeline
# --->8---
