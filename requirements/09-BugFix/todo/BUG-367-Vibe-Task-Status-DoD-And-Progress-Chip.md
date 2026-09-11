# BUG-367 — Vibe Task/CP must not be `done`; DoD boxes + task x/y UI

## Metadata

- Document ID: `BUG-367`
- Title: `Vibe Task/CP status must not be done; tick DoD boxes; show task x/y while coding`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-10`
- Last Updated: `2026-09-10`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md), [SS-18](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [BUG-365](./BUG-365-Vibe-Lock-No-SS-Stamp-And-Silent-Park.md), [CA-817](../../../change-audit/CA-817-Vibe-Sprint-Boundary-Continue-Gate.md)
- Replaces: `None`
- Tags: `vibe-mode, docs, dod, tui, live-bed`

## AI Quick View

### Summary

- Live run-654339 (`gate-sandbox` snake): vibe-sprint settled `done` after SS/CP/Tasks were authored and only Task-910 (core model) was coded. Task-911 tick/WASD and Task-912 playable `go run ./snake` were not implemented. Docs stayed `draft` with every DoD box open, and the TUI only showed `Flow: vibe-sprint` with no task 1/3 cue.
- Operator asked: (1) Task/CP document `status` must not be `done`; (2) DoD / Acceptance Check boxes must be ticked when a sprint finishes; (3) UI must show currently working on task x/y.
- Fix: engine stamps Task `in_progress` on sprint start and ticks DoD boxes on sprint complete (rewriting any `done` back to `in_progress`). CP lock stamps `approved` (never `done`) and last sprint ticks CP DoD. LoopState carries `vibeTaskIndex/Total/Name`; TUI composer + steps header render `task x/y`.

### Current Ask

- After a vibe sprint, Task/CP files are never `status: done`; completed DoD boxes are `[x]`; TUI shows `task 1/3` while coding.

### Key Decisions

- `V-1` `done` is reserved for a later human/ledger move into `requirements/**/done/`. Engine uses `approved` (CP lock) and `in_progress` (Task).
- `V-2` Tick only `- [ ]` under Acceptance Check / Definition of Done / DoD headings; Out of Scope stays open.
- `V-3` Progress is 1-based `vibeSprintIndex` / `len(plan)` and rides the existing agent-graph snapshot (no new endpoint).

### Constraints

- `feature_key: vibe-mode`.
- R1: additive tests only; old boundary / steps-header tests stay green (empty vibe fields hide the chip).
- R2: provider-agnostic (disk stamp + LoopState JSON; TUI matrixed Claude/Codex/Grok).

### Open Questions

- Remaining snake Tasks 911/912 still need a new vibe-sprint run; this bug does not auto-resume run-654339.

### Source Refs

- Live: run-654339 TUI screenshots 2026-09-10 (`vibe-sprint` done, synthesis “No production code”, `snake/game.go` + tests only, `main()` empty, CP-01/Tasks 910-912 `status: draft`, CP DoD all `[ ]`).
- Docs: `FORMAT-REFERENCE-TASK.md` Status enum, `FORMAT-REFERENCE-CP.md` DoD checkboxes, SS-13.

## 1. Issue Summary

The operator cannot see which Task a vibe sprint is on, and completed work does not tick DoD. Marking Task/CP `status: done` would lie: the file has not been moved to `done/` and sibling tasks may still be open.

## 2. Parent Links

- impacted coding plan: `CP-60` Branch V sprint loop
- impacted tech design: `SD-24`
- impacted system spec: `SS-18`, `SS-13` status vocabulary

## 3. Environment and Reproduction

- environment: TUI vibe-ingest → vibe-sprint on `/Users/tiendat/Desktop/BE/gate-sandbox`
- reproduction steps: snake prompt → lock SS → CP/Tasks sliced → one sprint codes `snake/` core model → flow `done`
- frequency: every multi-task vibe run (progress invisible; DoD never ticked)

## 4. Expected vs Actual

- expected: Task `in_progress`, CP `approved`, DoD `[x]` on finished items, TUI `task 1/3 Task-910-…` while coding
- actual: docs `draft`, DoD `[ ]`, composer `Flow: vibe-sprint · Thinking`, flow can settle `done` after sprint 1 of 3

## 5. Impact

- users affected: vibe TUI/Desktop operators tracking multi-task sprints
- workflows affected: `vibe-ingest` / `vibe-sprint`
- severity: medium (tracking), with live incomplete snake MVP

## 6. Root Cause

- hypothesis: no engine write-back for Task/CP/DoD after the SS lock stamp (BUG-365); LoopState had no task progress fields so TUI could not render x/y
- confirmed cause: `resumeVibeLock` stamped SS only; `maybeParkVibeSprintBoundary` / `maybeStartNextVibeSprint` did not touch docs; `AgentLoopState` had round/cap but not plan index
- evidence: CP-01 and Task-910/911/912 on disk after run-654339; `go test ./snake` 9 passed; no tick/input/render

## 7. Fix Strategy

- `F-1` `vibe_task_stamp.go`: rewrite status never to `done`; tick DoD boxes; CP lock → `approved`
- `F-2` Sprint start → Task `in_progress`; sprint complete / last sprint → tick Task (and CP on last) DoD
- `F-3` `AgentLoopState.vibeTaskIndex/Total/Name` on graph snapshot; TUI composer + steps header

## 8. Validation

- `V-1` New `TestBUG367_*` runner + TUI (3 providers for chips)
- `V-2` Old `TestStepsHeader_ShowsRoundChip` still exact `steps  round 2/3`; boundary park tests untouched and green

## 9. Regression Guard

- tests: `runner/bug367_vibe_task_status_dod_test.go`, `tui/app/bug367_vibe_task_progress_chip_test.go`
- alerts: none
- audit checks: CA-822

## 10. Follow-Up Document Updates

- upstream docs that must change: none (SS-13 status vocabulary unchanged)
- notes left unchanged on purpose: CA-817 boundary gate, BUG-365 SS stamp, Task-322 round chip
