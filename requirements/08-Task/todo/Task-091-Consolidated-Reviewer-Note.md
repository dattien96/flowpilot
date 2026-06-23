# Task-091: Consolidated Reviewer Note

## Metadata

- Document ID: `Task-091`
- Title: `Consolidated Multi-Reviewer Result Note`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-090: Review Loop Driver](./Task-090-Review-Loop-Driver.md), [Task-092: Auto Reinvoke Orchestrator](./Task-092-Auto-Reinvoke-Orchestrator.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, result-sharing, go`

## AI Quick View

### Summary

- Group a round's reviewers into a cohort (`reviewCohortId`) and, when all cohort members are terminal, fold their results into ONE consolidated, per-reviewer-labelled note delivered via the existing `pendingAgentContext` channel.
- This gives the synthesizing agent one clean input that names each reviewer's findings (and any failures), instead of N ungrouped lines.

### Current Ask

- Implement cohort tracking + the single consolidated note builder, reusing `pendingAgentContext` so it folds into the next turn and survives restart.

### Key Decisions

- `T-1` The note is built and appended exactly once, only after the LAST cohort member reaches a terminal state (SD-18 `D-5`).
- `T-2` A failed reviewer is included in the note as `failed: <error>`; synthesis proceeds with the rest (SS-15 `E-1`).

### Constraints

- Run GitNexus impact analysis before editing the completion handler (`interactive_service.go:870-998`) and `appendPendingAgentContextLocked`; warn on HIGH/CRITICAL.
- Reuse `pendingAgentContext` (SD-16 §14.3); do not invent a new channel. Truncate each reviewer result (≈1500 chars).

### Open Questions

- None.

### Source Refs

- CP-36 `P-3`; SD-18 `D-5`, §6 (result-sharing), §7.5; SS-15 `AC-2`, `E-1`.
- Anchors: `provider_registry.go` (`interactiveRun` fields); `interactive_service.go:870-998` (completion handler, `appendPendingAgentContextLocked`), `:394-408` (cohort-completeness pattern via `dependenciesSatisfiedLocked`).

## 1. Goal

When the reviewers of one round all finish, the orchestrator's next turn receives a single labelled summary of every reviewer's findings, ready for synthesis.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-3`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-5`, §7.5
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-2`
- specific upstream ids: CP-36 `P-3`

## 3. Trigger

With multiple reviewers, the parent currently sees N separate result lines and no grouping or conflict signal. Synthesis needs one consolidated, attributable input.

## 4. Exact Change

- `T-1` Add `reviewCohortId` to `SpawnAgentInput` (`json:"reviewCohortId,omitempty"`) and to the child `interactiveRun` (additive field).
- `T-2` Add an orchestrator cohort buffer `cohortResults map[string][]cohortEntry` (`{runId, agentName, provider, finalMessage|error, status}`); on each cohort child's `EventTurnCompleted`/`EventTurnFailed`, append to the buffer instead of appending an isolated `pendingAgentContext` line.
- `T-3` Cohort-completeness: when every child with `reviewCohortId==X` is terminal (`completed|failed|cancelled`), build ONE note ("Review round R — N reviewers reported" + one labelled block per reviewer + a synthesis directive) and append via `appendPendingAgentContextLocked`. Truncate each result.
- `T-4` Persist `reviewCohortId` in the session snapshot (`interactive_service.go:744-762`) and restore on resume.

## 5. Touched Areas

- files: `agent_orchestrator.go` (`SpawnAgentInput`, cohort buffer), `provider_registry.go` (`interactiveRun`), `interactive_service.go` (completion handler, note builder, snapshot), `interactive_resume.go`.
- modules: local-runner orchestration + interactive service.
- routes: none.
- tables: none.

## 6. Acceptance Check

- Unit: 3 cohort children completing out of order produce exactly ONE consolidated note, only after the last completes, containing all three labelled blocks.
- A failed reviewer appears as `failed: <error>` and the note is still produced.
- The note folds into the parent's next provider turn (existing `pendingAgentContext` behavior) and survives a restart.

## 7. Out of Scope

- Triggering the synthesis turn automatically (Task-092); the verdict/loop (Task-090); UI (Task-094).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
