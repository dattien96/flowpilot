---
id: CA-513
feature_key: cli-tui
title: Show F2 step [open] live when child agents start
date: 2026-08-14
status: COMPLETE
---

## Change

While a flow is running, users must open a child agent transcript as soon as
that agent starts — not only after the whole run finishes or after `/open`.

### Root cause

F2 step `[open]` requires `agentRuns` (`childRunForStep`). Live flow only polled
**steps-runtime**; agent graph was hydrated mainly on `/open` or when an
`agent_graph_updated` SSE arrived. Missed/late SSE left steps RUNNING with no
`[open]`. Collapsed F2 also hid the control when children finally appeared.

### Fix (TUI-only)

1. **Poll** `cmdHydrateAgentRuns` with steps-runtime (~1.6s) while
   `shouldPollStepsRuntime()`.
2. **Hydrate on run start** (flow/catalog) and when steps transition into an
   agent-bearing RUNNING/DONE/FAILED state (`stepsSuggestChildAgentOpen`).
3. **Expand F2** (`sessionPanel.Collapsed = false`) whenever child agents are
   present (hydrate, `agent_graph_updated`, `AgentGraphMsg`, steps refresh).

Opening still uses existing `/agent` focus stream (live `StreamLive` after
resume seed) — no runner change.

### Provider impact

Case 1 (agnostic). Uses existing `GET …/workflow-runs/{id}/agents` + steps
runtime for Claude / Codex / Grok flows.

### Prior claims kept

CA-499 open mapping; CA-506 F2-only open/back; CA-504 live child transcript;
CA-508 no false [stop] on completed open.

### Tests

New `tui_live_agent_open_test.go` (hydrate expand + open, graph event expand,
steps transition hydrate, heuristics). Old suite untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: task
summary: Live flow polls agent hydrate and expands F2 so step [open] shows when child agents start
# --->8---
