---
id: CA-499
feature_key: cli-tui
title: TUI step [open] and /agents picker for child transcripts
date: 2026-08-14
status: COMPLETE
---

## Change

Flow-mode session panel lists steps top-right. A step that maps to a spawned
child agent (`AgentRef` / `NodeID` / `StepType` vs `agentRuns`, never main)
now shows clickable `[open]` (same as `/agent <name>`). Child view shows
`[back]` on the panel and status row to restore main.

`/agents` and `/agent ` open a Tab/Enter picker of live agents when the graph
has runs. Bare `/agents` with no children still toggles Agents focus (CA-443
surface). `/agents <name>` aliases `/agent <name>`.

Two open paths only: click `[open]`, or `/agents`/`/agent` (+ Tab). No extra
runner / startTurn change.

## Provider impact

Case 1 (agnostic). UI maps runner `agent_graph` + steps-runtime only.

## Tests

New `tui_step_agent_open_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI flow steps get [open] to child transcript; /agents picker + [back] to main
# --->8---
