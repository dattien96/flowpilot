# BUG-371 — Composer [stop] remains after vibe flow is done

## Metadata

- Document ID: `BUG-371`
- Title: `Composer still shows [stop] when the loop is done (task 3/3 · [stop] · done)`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-60](../../07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [CA-537](../../../change-audit/), run-189839 settle chrome
- Replaces: `None`
- Tags: `cli-tui, vibe-mode, live-bed`

## AI Quick View

### Summary

- Live run-678326: chrome `Flow: vibe-ingest · task 3/3 · [stop] · done` after synthesis `submit_review_outcome → approved`. All steps checked. [stop] is live-work chrome and must drop.
- Cause: `turnIsActive` (composer [stop]) did not treat a done loop with no RUNNING child as idle. `workIsLive` already did. `flowHasActiveAgents` still counted `waiting_user_approval` as active; leftover `flowStepsActive` also kept `flowLoopDone()` false.
- Fix: if `flowLoopStatus==done` and `!hasLiveWorkingChild()`, `turnIsActive` is false. Ask_user/approval/gate overlays also hide [stop]. A done loop with a still-RUNNING child still arms [stop] (old `TestFlowDone_NotSettledWhileChildRunning`).

### Current Ask

- Done loop + no RUNNING child → composer has no [stop].

### Key Decisions

- `V-1` Do not use raw `flowLoopDone()` alone (it is false when `flowStepsActive` is stale).
- `V-2` Keep [stop] when a child is actually RUNNING.

### Constraints

- `feature_key: cli-tui` (composer) / `vibe-mode` live. Dominant: cli-tui.
- R1: additive TUI tests; old settle tests green untouched except we must not invert live-child [stop].
- R2: TUI matrix Claude/Codex/Grok.

### Open Questions

- None.

### Source Refs

- TUI 2026-09-11 screenshot: `task 3/3 · [stop] · done`

## 1. Issue Summary

Operator cannot tell the run is finished; [stop] implies a live turn.

## 2. Parent Links

- impacted coding plan: CP-60
- impacted tech design: SD-24
- impacted system spec: SS-18

## 3. Environment and Reproduction

- environment: TUI after vibe-ingest last audit
- reproduction steps: loop status done, leftover waiting child or orch listener
- frequency: every settled vibe run that still has a parked child stamp

## 4. Expected vs Actual

- expected: `Flow: vibe-ingest · task 3/3 · done`
- actual: `[stop]` between task chip and done

## 5. Impact

- users affected: TUI flow operators
- workflows affected: any flow that settles done
- severity: medium (chrome)

## 6. Root Cause

- hypothesis: turnIsActive vs workIsLive disagree
- confirmed cause: turnIsActive used flowHasActiveAgents (includes waiting_user_approval) and orch+non-terminal handle
- evidence: `chatFrameTitle` gates [stop] on `turnIsActive`; screenshot

## 7. Fix Strategy

- `F-1` `turnIsActive`: done + no live working child → false
- `F-2` question/approval/gate → false (ask_user is not a stoppable turn)

## 8. Validation

- `V-1` New `TestBUG371_*` (3 providers + ask_user + live child still stop)
- `V-2` `TestFlowDone_NotSettledWhileChildRunning` still wants [stop]

## 9. Regression Guard

- tests: `tui/app/bug371_stop_hidden_when_loop_done_test.go`
- alerts: none
- audit checks: CA-826

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: run-189839 settle path
