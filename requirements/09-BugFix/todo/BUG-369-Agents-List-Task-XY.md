# BUG-369 — /agents list cannot tell which vibe task a duplicate agent belongs to

## Metadata

- Document ID: `BUG-369`
- Title: `/agents` picker repeats the same names (3× coder, 3× preflight_contract_plan) with no task x/y`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-10`
- Last Updated: `2026-09-10`
- Parent Documents: [CP-60](../../07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md), [BUG-367](./BUG-367-Vibe-Task-Status-DoD-And-Progress-Chip.md)
- Child Documents: `None`
- Related Documents: [CA-822](../../../change-audit/CA-822-Vibe-Task-DoD-And-Progress-Chip.md)
- Replaces: `None`
- Tags: `vibe-mode, cli-tui, agents, live-bed`

## AI Quick View

### Summary

- Live vibe-ingest `/agents` picker listed three `preflight_contract_plan` and multiple `coder` rows with only status + run id. Operator could not tell which sprint/task each child belonged to.
- Composer already shows `task x/y` (BUG-367) on the parent loop; child summaries did not carry a per-spawn snapshot.
- Fix: stamp `vibeTaskIndex/Total/Name` on the child at spawn (from the parent's plan at that moment), keep it across summary upserts, show `task 2/3 Task-911.md` in the `/agents` picker detail, dump, and sidebar.

### Current Ask

- `/agents` rows for vibe-sprint children include `task x/y` plus the short Task file name.

### Key Decisions

- `V-1` Stamp at spawn, not at display time — otherwise every historical coder would show the live index (all 3/3).
- `V-2` `upsertSummary` keeps existing vibe fields when the incoming update omits them (turn-progress rebuilds).
- `V-3` Ingest/lock children with index 0 show no chip.

### Constraints

- `feature_key: vibe-mode`.
- R1: additive tests only.
- R2: agnostic; TUI chips matrixed Claude/Codex/Grok.

### Open Questions

- None.

### Source Refs

- TUI screenshot 2026-09-10 `/agents` picker, run-678326 family.

## 1. Issue Summary

Sequential vibe-sprints reuse node ids (`coder`, `tdd`, `preflight_contract_plan`). The picker left column is that id; the right column was only status · agent · run id.

## 2. Parent Links

- impacted coding plan: CP-60
- impacted tech design: SD-24
- impacted system spec: SS-18

## 3. Environment and Reproduction

- environment: TUI vibe-ingest, `/agents` after two+ sprints
- reproduction steps: run snake ingest through sprint 2, open `/agents`
- frequency: every multi-task vibe run

## 4. Expected vs Actual

- expected: `preflight_contract_plan` detail includes `task 2/3 Task-911.md`
- actual: three identical names, only run ids differ

## 5. Impact

- users affected: vibe TUI operators
- workflows affected: vibe-ingest / vibe-sprint
- severity: medium (control, not correctness)

## 6. Root Cause

- hypothesis: AgentRunSummary has no per-child task fields
- confirmed cause: spawn upsert copies name/label/status only
- evidence: screenshot; `filterAgentSuggestions` detail builder

## 7. Fix Strategy

- `F-1` Stamp parent `vibeTaskProgress` onto the child at spawn
- `F-2` Persist on child session; merge on upsert
- `F-3` Picker / dump / sidebar render `task x/y`

## 8. Validation

- `V-1` New `TestBUG369_*` runner + TUI
- `V-2` Old `/agents` picker tests untouched and green

## 9. Regression Guard

- tests: `runner/bug369_agent_task_stamp_test.go`, `tui/app/bug369_agents_task_chip_test.go`
- alerts: none
- audit checks: CA-824

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: BUG-367 parent chrome `task x/y`
