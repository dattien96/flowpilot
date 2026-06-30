# Task-093: Bounded Auto-Reinvocation Of The Hub

## Metadata

- Document ID: `Task-093`
- Title: `Bounded Auto-Reinvocation Of The Hub (Opt-In, Single-Flight, Cap-Bounded)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [Task-092: Consolidated Multi-Result Join Note](./Task-092-Consolidated-Multi-Result-Join-Note.md), [Task-090: Bounded Flow Runtime Executor](./Task-090-Bounded-Flow-Runtime-Executor.md)
- Replaces: `None`
- Tags: `multi-agent, flow-engine, auto-reinvoke, bounded, single-flight, go`

## AI Quick View

### Summary

- Add the runner primitive that re-prompts the hub automatically after a cohort `join` completes, so the loop advances without a human typing — the concrete build of SD-16 §14.5 "auto agent-to-agent mode."
- Opt-in per run (`autoOrchestrate`), single-flight, status-gated, and bounded by the cap; cancelled by Stop. A run without the flag (normal chat) never auto-reinvokes.
- The re-prompt uses the system-note prepend path (the Task-092 join note + synthesis directive), never a user bubble.

### Current Ask

- Implement `maybeAutoReinvokeHub` with all guards + single-flight + Stop-cancel + resume persistence, and prove it never fires on a normal chat run.

### Key Decisions

- `T-1` `autoOrchestrate` is set when a flow starts (skill's first spawn carries it, or the start/flow-control path sets it) and persisted in `sessions.ndjson` (via Task-085) so resume keeps the loop alive.
- `T-2` Re-invoke fires **once per cohort join**; the hub's `flow_control` then advances (new cohort → next reinvoke) or ends; cap + `blocked` are hard stops (CP-36 `R-2`).
- `T-3` The re-prompt schedules a **parent** turn (`scheduleChildTurn(parentRunID,...)`) via the system-note path (SD-16 §14.3), never a user bubble.

### Constraints

- Run GitNexus impact analysis before editing `interactive_service.go`, `provider_registry.go`, `agent_orchestrator.go`; warn on HIGH/CRITICAL.
- The loop MUST remain bounded and stoppable; an unbounded re-prompt is a release blocker.
- A run without `autoOrchestrate` behaves exactly as today (normal chat regression guard).

### Open Questions

- None.

### Source Refs

- CP-36 `P-7`, `R-2`; SD-18 `D-4`, §7; SD-16 §14.3/§14.5.
- Anchors: `interactive_service.go:394-571` (`releaseDependentAgents`, `scheduleChildTurn`), `:356` (`stopAgentLoop`); `provider_registry.go` (`interactiveRun.autoOrchestrate`, `turnInFlight`); `agent_orchestrator.go` (loop status, `reinvokeInFlight`).

## 1. Goal

After a cohort join completes, the runner re-prompts the hub once (opt-in, single-flight, cap-bounded, Stop-cancellable) so the flow advances autonomously; normal chat runs are never auto-reinvoked.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-7`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-4`, §7; [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) §14.5
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-6`
- specific upstream ids: CP-36 `P-7`; SD-18 `D-4`

## 3. Trigger

The synchronous loop (Task-090/091/092) requires the hub to be re-prompted after each cohort completes; doing that automatically (not by a human typing) is what makes the parallel review experience work, and it must be bounded.

## 4. Exact Change

- `T-1` Add `autoOrchestrate bool` to the parent `interactiveRun` (additive). Set true when a flow starts (the skill's first spawn carries `SpawnAgentInput.AutoOrchestrate`, or the flow-control/start path sets it). Persist in `sessions.ndjson` (Task-085).
- `T-2` Add `reinvokeInFlight bool` per parent (single-flight guard).
- `T-3` `maybeAutoReinvokeHub(parentRunID string)` called at the cohort-complete branch (Task-092) and from `releaseDependentAgents`. Guards (ALL required):
  1. `autoOrchestrate == true`.
  2. Loop `Status ∉ {paused, stopped, blocked, done}`.
  3. `!parent.turnInFlight`.
  4. `!reinvokeInFlight` (set true for the duration).
  5. `Round < Cap`.
- `T-4` Action: `scheduleChildTurn(parentRunID, parentStepID, synthesisPrompt)` where `synthesisPrompt` is the Task-092 join note + synthesis directive, via the **system-note** prepend path (SD-16 §14.3) — never a user bubble.
- `T-5` `stopAgentLoop` (`:356`) clears `autoOrchestrate` + `reinvokeInFlight`; clear `reinvokeInFlight` when the scheduled turn starts.

## 5. Touched Areas

- files: `interactive_service.go`, `provider_registry.go`, `agent_orchestrator.go`; tests `interactive_service_test.go`, `agent_orchestrator_test.go`.
- modules: local-runner flow engine.
- routes: none new.
- tables: none (flag persisted in `sessions.ndjson` via Task-085).

## 6. Acceptance Check (Definition of Done)

- [ ] With `autoOrchestrate` on, a completed cohort join triggers **exactly one** hub re-prompt (single-flight verified under concurrent completions).
- [ ] Re-invoke does **not** fire when the loop is `paused`, `stopped`, `blocked`, `done`, at cap, or while a turn is in flight.
- [ ] The re-prompt uses the system-note path (no user bubble appears in the transcript).
- [ ] Pressing **Stop** cancels a pending reinvoke and clears `autoOrchestrate`/`reinvokeInFlight`.
- [ ] A run **without** `autoOrchestrate` (normal chat) **never** auto-reinvokes (regression guard).
- [ ] After a restart, a flow with `autoOrchestrate` resumes and continues auto-reinvoking at the correct round (with Task-085).

## 7. Out of Scope

- The join note content (Task-092); the executor/cap (Task-090); the review tool/config (Task-091); the skill that sets the flag (Task-094); UI (Task-095).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
