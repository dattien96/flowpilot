# BUG-364 — Slicer park skips step DONE (settle/park self-poisoning order)

## Metadata

- Document ID: `BUG-364`
- Title: `Slicer park skips step DONE — tryAdvance parks before stamping the completed node (run-640953)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-09`
- Last Updated: `2026-09-09`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md), [CP-60 Test Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md)
- Child Documents: `None`
- Related Documents: [BUG-363](./BUG-363-Vibe-Ingest-Writer-Output-Unverified.md), [SS-18](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md)
- Replaces: `None`
- Tags: `vibe-mode, settlement, fail-closed, live-bed, regression`

## AI Quick View

### Summary

- Live run-640953: `task_slicer` child (run-641369) completed 22:51:43 writing zero files; BUG-363 park fired correctly at 22:51:42.648 (loop `blocked/requirement`, byte-identical gate reason) — but the flow step stayed RUNNING for 7+ min with no card and no watchdog.
- Root cause is ordering inside `tryAdvanceFlowFromNode` (`flow_executor.go`): `onVibeCpNodeDone` (which now parks, CA-817) runs at `:1160` BEFORE the `loopIsAdvancing` gate (`:1166`) and before every DONE write (`:1203-1206`, `:1273-1275`). The park trips its own downstream gate — self-poisoning.
- Fix: stamp the completed slicer node DONE (mirroring the cohort self-settle and the `:1203` terminal write) before `parkVibeRequirement`, in the BUG-363 branch.
- NOT the cause: settle dispatch (correct — `flow_advance_skipped_loop_blocked` is only reachable inside `tryAdvance`), hub liveness (main completed 22:49:36 yet earlier advances succeeded), BUG-363 intent (fail-closed decision correct, trigger correct).

### Current Ask

- Stamp DONE-then-park in the slicer branch; additive ordering tests through settle→advance (not just direct `onVibeCpNodeDone` calls — BUG-363 tests missed this ordering by construction).

### Key Decisions

- `V-1` Fix at the BUG-363 branch (stamp before park), NOT by hoisting the DONE write above `:1160` in `tryAdvance` — hoisting changes every node type's path (blast radius); the branch fix is vibe-slicer-scoped (R1-safe).
- `V-2` TUI card surfacing for requirement parks is OUT of scope (separate concern; the loop GateReason already carries the text). This bug is the step terminal state only.
- `V-3` Same R1 carve-outs as BUG-363: empty-cwd legacy shapes and CA-783/786/791 pins untouched.

### Constraints

- `feature_key: vibe-mode`.
- R1: no edits to old tests; the new ordering test must FAIL on CA-817 code and PASS after fix.
- R2: agnostic (step-status write + disk glob, no `providerKey`).
- Review bar per operator: 3 turns × 2 agents (this bug class recurs).

### Open Questions

- None blocking. Follow-up candidate: emit a user-visible card on requirement parks (surfacing gap noted live).

### Source Refs

- Live: run-640953, child run-641369 (doc-writer/task_slicer running 22:51:27 → completed 22:51:43), `~/.flowpilot/tui.log` (`StepsRuntimeMsg` RUNNING stalled, `agentRunsHydratedMsg` completed).
- Diag: `flow_parked_awaiting_user` 22:51:42.648 with BUG-363 gate reason; `flow_advance_skipped_loop_blocked completed_node_id=task_slicer` 22:51:42.651; `hub_reinvoke_blocked`; zero `hub_stalled` events (blocked loops exempt by design, `hub_stall.go:317-320,386-393`).
- Binary built 22:48 from source including CA-817 (commits 22:41) — fix was live; park fired as designed.

## 1. Issue Summary

A correctly-decided fail-closed park leaves its own flow step RUNNING forever because the park runs before the step-DONE write in `tryAdvanceFlowFromNode`. The TUI clock keeps ticking on a stale RUNNING step with no card and no watchdog — indistinguishable from a hung agent, but the agent finished fine.

## 2. Parent Links

- impacted coding plan: `CP-60-Vibe-Working-Mode.md`; `CP-60-Test-Steps.md` V4/V5 (live validation pending).
- impacted tech design: `SD-24-Vibe-Working-Mode.md` (slicer → sprint chain).
- impacted system spec: `SS-18-Vibe-Working-Mode.md`.

## 3. Environment and Reproduction

- environment: TUI `just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` (Mac), provider opencode/muse-spark, branch `cp60-vibe` at CA-817.
- reproduction steps: clean sandbox → new `/vibe` snake run → lock SS → `cp_writer` writes non-CP artifact → `task_slicer` completes writing zero Task files → (with CA-817 live) loop parks but step stays RUNNING.
- frequency: observed once live (run-640953); deterministic at unit level through settle→advance ordering.

## 4. Expected vs Actual

- expected: slicer DONE + loop `blocked/requirement` with the BUG-363 gate reason (fail-closed AND terminally settled).
- actual: loop parked correctly, step stuck RUNNING, no terminal state, no card, no watchdog.

## 5. Impact

- users affected: operator live bed (run-640953 stalled visibly; V4/V5 untickable).
- workflows affected: any vibe slicer completion that parks (BUG-363 path); pattern risk for any future park-inside-`onVibeCpNodeDone`.
- severity: medium (visible stall, no data loss; recovery = new run after fix).

## 6. Root Cause

- hypothesis (settle skips write/advance): REFUTED — `flow_advance_skipped_loop_blocked` proves settle dispatched into `tryAdvance`.
- hypothesis (advance needs live hub): REFUTED — ingest→converter (22:50:24), cohort join + reinvoke (22:50:48), ss_lock→cp_writer (22:51:07) all advanced after main completed 22:49:36.
- confirmed cause: `tryAdvanceFlowFromNode` calls `onVibeCpNodeDone` (`flow_executor.go:1160`) before the `loopIsAdvancing` gate (`:1166`) and the DONE writes (`:1203-1206`, `:1273-1275`). CA-817's park trips the gate 3ms later; DONE writes never reached. Terminal `task_slicer --done--> done` would have been stamped at `:1203` had the loop still been advancing.
- evidence: code order + live diag timestamps (park 22:51:42.648 → skip 22:51:42.651); BUG-363 unit tests called `onVibeCpNodeDone` directly so they could not see this ordering (test-shape gap, fixed by the new ordering test).

## 7. Fix Strategy

- `F-1` In the BUG-363 park branch (`vibe_cp.go` slicer case): `setFlowStepStatus(ctx, parentRunID, completedNodeID, StepStatusDone)` BEFORE `parkVibeRequirement` — mirrors the cohort self-settle (`interactive_service.go:5009-5013`) and the `:1203` terminal write. Unlocked variant (no `s.mu` held at that point, CA-811-compliant).
- `F-2` Additive ordering test (`bug364_slicer_park_stamps_done_test.go`): drive completion through `settleFlowChildTurnCompletedLocked` with `flowCohortId == ""` on the overlaid vibe graph; assert step DONE + loop `blocked/requirement`. Must FAIL on CA-817 code, PASS after fix.
- `F-3` Explicitly NOT doing: hoisting DONE above `:1160` (blast radius), TUI card for parks (separate bug), touching the `:1166` gate (BUG-234 protection).

## 8. Validation

- `V-1` New `bug364_*` ordering test red-before/green-after; `bug363_*` still green.
- `V-2` Contract subset (`TestBUG36*`, CA-78x/79x, `TestOnVibe*`, `TestCollect*`, `TestAdvanceHubDone*`, Task-321/326) green, 0 FAIL.
- `V-3` Live re-run must show slicer DONE + parked gate reason (operator tick, not claimed here).

## 9. Regression Guard

- tests: `bug364_slicer_park_stamps_done_test.go` (ordering through settle; park-reason preserved; empty-cwd legacy unaffected).
- alerts: any `bug36*` / CA-78x/79x red ⇒ STOP (R1).
- audit checks: CA-818 note with ordering evidence + will-not-undo (BUG-234 gate, CA-817 park, cohort settle).

## 10. Follow-Up Document Updates

- upstream docs that must change: `change-audit/CA-818` (new note); CP-60-Test-Steps §7 evidence after live re-run.
- notes left unchanged on purpose: CA-817 (park decision stays), CA-783 (SS fallback stays), BUG-234 gate (`:1166` stays), `tryAdvance` head order (`:1160` stays).
