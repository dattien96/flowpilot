# Task-092: Auto Reinvoke Orchestrator

## Metadata

- Document ID: `Task-092`
- Title: `Opt-In Auto-Reinvocation Of The Orchestrator`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-090: Review Loop Driver](./Task-090-Review-Loop-Driver.md), [Task-091: Consolidated Reviewer Note](./Task-091-Consolidated-Reviewer-Note.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, auto-orchestration, concurrency, go`

## AI Quick View

### Summary

- Add an opt-in `autoOrchestrate` flag so the runner re-prompts the orchestrating agent once a reviewer cohort completes — advancing the loop without the user typing.
- The re-prompt is single-flight, bounded by the cap, suppressed while a turn is in flight or the loop is paused/stopped/blocked, and cancelled by Stop. Normal runs (flag off) never auto-reinvoke.

### Current Ask

- Implement `maybeAutoReinvokeOrchestrator` with all guards + the `autoOrchestrate` lifecycle; this is the riskiest slice and must be provably bounded.

### Key Decisions

- `T-1` The re-prompt seeds a PARENT turn with the consolidated note (Task-091) + synthesis directive via the non-user-visible system-note path (SD-16 §14.3) — never a user bubble.
- `T-2` Guards (ALL required): flag on, loop runnable, no parent turn in flight, single-flight (`reinvokeInFlight`), round < cap (SD-18 `D-3`).

### Constraints

- Run GitNexus impact analysis before editing the completion/release path (`interactive_service.go:448-571,989-998`); warn on HIGH/CRITICAL.
- The loop MUST be provably bounded and stoppable — a runaway re-prompt loop is a release blocker.
- A run without `autoOrchestrate` must behave exactly as today (normal-chat regression guard).

### Open Questions

- None.

### Source Refs

- CP-36 `P-4`; SD-18 `D-3`, §8 `F-3`/`F-4`/`F-7`, `R-1`; SS-15 `AC-5`, `AC-9`, `AC-10`, `BR-4`.
- Anchors: `interactive_service.go:448-525` (`releaseDependentAgents`), `:527-571` (`resumePendingLoopWork`), `:356-375` (`stopAgentLoop`), `:989-998` (completion hook); `provider_registry.go` (`interactiveRun` flags).

## 1. Goal

After a reviewer cohort completes, the orchestrating agent is automatically re-prompted to synthesize and submit a verdict — bounded, single-flight, and stoppable — so the loop runs to a clean result or a user gate without manual re-prompting.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-4`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-3`, `R-1`
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-5`
- specific upstream ids: CP-36 `P-4`

## 3. Trigger

Tasks 090–091 produce a consolidated note but nothing re-prompts the orchestrator, so the loop stalls until the user types. Auto-reinvocation closes the loop (SS-15 `AC-5`, the concrete build of SD-16 §14.5).

## 4. Exact Change

- `T-1` Add `autoOrchestrate bool` + `reinvokeInFlight bool` to the parent `interactiveRun`; persist `autoOrchestrate` in the session snapshot and restore on resume. Set it true when the review loop starts (skill's first cohort spawn / `review-outcome` start path).
- `T-2` Implement `maybeAutoReinvokeOrchestrator(parentRunID)`, called at the end of the cohort-complete branch (Task-091) and from `releaseDependentAgents`. Apply all guards (Quick View `T-2`). Extend `loopAllowsNextTurnLocked` to also block on `blocked`/`approved`/`rejected`.
- `T-3` On pass, schedule a parent turn seeded with the consolidated note + synthesis directive (reuse the `pendingAgentContext` prepend path); set `reinvokeInFlight=true`; clear it when that turn completes.
- `T-4` `stopAgentLoop` clears `autoOrchestrate` and `reinvokeInFlight`; add `[review-loop] reinvoke fired|suppressed:<reason>` logs.

## 5. Touched Areas

- files: `provider_registry.go` (`interactiveRun` flags), `interactive_service.go` (`maybeAutoReinvokeOrchestrator`, guards, `stopAgentLoop`, completion hook), `interactive_resume.go` (persist/restore flag).
- modules: local-runner orchestration + interactive service.
- routes: none new (reuses Task-090 routes).
- tables: none.

## 6. Acceptance Check

- Unit: reinvoke fires exactly once when a cohort completes with the flag on; does NOT fire when paused/stopped/blocked/at-cap/turn-in-flight; Stop cancels a pending reinvoke; a run without the flag never auto-reinvokes.
- Manual: a multi-round loop advances coder→reviewers→synthesis→verdict→coder with no user typing, and halts at cap/Stop.
- Restart mid-loop preserves `autoOrchestrate` and continues.

## 7. Out of Scope

- The verdict/loop machine (Task-090); the note builder (Task-091); UI (Task-094).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
