# Task-092: Consolidated Multi-Result Join Note

## Metadata

- Document ID: `Task-092`
- Title: `Consolidated Multi-Result Join Note (Cohort Barrier Deliver)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [Task-090: Bounded Flow Runtime Executor](./Task-090-Bounded-Flow-Runtime-Executor.md), [Task-093: Bounded Auto-Reinvocation Of The Hub](./Task-093-Bounded-Auto-Reinvocation-Of-The-Hub.md)
- Replaces: `None`
- Tags: `multi-agent, flow-engine, join, barrier, consolidated-note, go`

## AI Quick View

### Summary

- Turn N children completing into **one** consolidated, per-member-labelled note delivered to the hub when a node's `join` condition is met — the generic "deliver" half of the barrier.
- Add `flowCohortId` to `SpawnAgentInput` + child `interactiveRun`; accumulate cohort results instead of appending one isolated line per child; emit a single note + synthesis directive via `appendPendingAgentContextLocked` once the cohort is complete.
- Out-of-order completion safe; a failed child is included as `failed: <error>`.

### Current Ask

- Implement the cohort buffer + completeness check + single consolidated note, replacing the per-child isolated append, with out-of-order and failed-child unit tests.

### Key Decisions

- `T-1` Cohort grouping is generic (`flowCohortId`), not review-specific; member labels come from the spawn config, so the note carries no hardcoded role names (CP-36 `P-1`).
- `T-2` The note is appended as a **single** `pendingAgentContext` entry so it folds into the hub's next turn (SD-16 §14.3) and persists in `sessions.ndjson` (survives restart via Task-085).
- `T-3` Cohort completeness reuses `dependenciesSatisfiedLocked`-style logic: complete when every child with `flowCohortId==X` is terminal (`completed|failed|cancelled`) and the node's `Join` is satisfied.

### Constraints

- Run GitNexus impact analysis before editing `agent_orchestrator.go`, `interactive_service.go`, `provider_registry.go`; warn on HIGH/CRITICAL.
- Reuse `pendingAgentContext`; do not invent a new channel. Truncate each member result to 1500 chars (token guard, SD-18 `R-4`).
- Additive; single-child/no-cohort runs keep the existing isolated-append behavior.

### Open Questions

- None.

### Source Refs

- CP-36 `P-6`; SD-18 `D-5`, §6; SD-16 §14.3.
- Anchors: `interactive_service.go:394-571` (`dependenciesSatisfiedLocked`), `:870-879,944-947` (current per-child append), `appendPendingAgentContextLocked`; `provider_registry.go` (`interactiveRun` fields near `:103-110`); `agent_orchestrator.go` (cohort buffers).

## 1. Goal

When a cohort of children feeding a join node all complete, deliver one consolidated, labelled note (plus a synthesis directive) to the hub, replacing N ungrouped lines — the generic barrier "deliver" that the review loop and any future fan-in flow reuse.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-6`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-5`, §6; [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) §14.3
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-5`
- specific upstream ids: CP-36 `P-6`; SD-18 `D-5`

## 3. Trigger

Today `appendPendingAgentContextLocked` appends one line per child; with N reviewers the hub sees N ungrouped lines and no conflict signal. The engine needs a single grouped deliver so the hub can synthesize in one turn.

## 4. Exact Change

- `T-1` Add `flowCohortId string` to `SpawnAgentInput` (`json:"flowCohortId,omitempty"`) and to the child `interactiveRun` (additive, near `:103-110`).
- `T-2` Add an orchestrator cohort buffer `cohortResults map[string][]cohortEntry` (`cohortEntry{label, provider, finalMessage, status, err}`).
- `T-3` On a cohort child's `EventTurnCompleted` (`interactive_service.go:870-879,944-947`): if `flowCohortId != ""`, append to the buffer instead of the isolated `pendingAgentContext` line.
- `T-4` Completeness: when every child with `flowCohortId==X` is terminal and the join node's `Join` is satisfied (`joinSatisfied`), build ONE note:
  ```
  [FlowPilot flow round R — N results joined]
  "<label>" (<provider>): <finalMessage truncated 1500>
  ...
  ---
  Synthesize: dedup the findings, flag any conflicting verdicts, resolve them using the
  task context, then call the flow's control tool with the consolidated result.
  ```
  Append via `appendPendingAgentContextLocked` as a **single** entry. A failed child is included as `"<label>" (<provider>): failed: <error>`.
- `T-5` Single-child / no-`flowCohortId` runs keep the existing isolated-append path (no behavior change).

## 5. Touched Areas

- files: `agent_orchestrator.go`, `interactive_service.go`, `provider_registry.go`; tests `agent_orchestrator_test.go`, `interactive_service_test.go`.
- modules: local-runner flow engine.
- routes: none.
- tables: none (note rides in `sessions.ndjson` via Task-085).

## 6. Acceptance Check (Definition of Done)

- [ ] Three children in one cohort completing **out of order** produce **exactly one** consolidated note, emitted only after the last completes.
- [ ] The note contains all members, each prefixed by its config-supplied `label` + provider, truncated to 1500 chars.
- [ ] A **failed** reviewer is included as `failed: <error>`; synthesis still proceeds.
- [ ] The note is a **single** `pendingAgentContext` entry (folds into the hub's next turn) and is present after a restart (with Task-085).
- [ ] The note carries **no hardcoded role names** — labels come from spawn config (domain-free).
- [ ] A single-child / no-`flowCohortId` run keeps the existing isolated-append behavior (regression).

## 7. Out of Scope

- Triggering the hub turn from the note (Task-093 auto-reinvoke); the executor/join primitive itself (Task-090); the review tool/config (Task-091); persistence wiring (Task-085); UI (Task-095).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
