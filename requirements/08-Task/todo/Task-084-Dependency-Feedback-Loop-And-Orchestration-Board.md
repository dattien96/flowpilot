# Task-084: Dependency Feedback Loop And Orchestration Board

## Metadata

- Document ID: `Task-084`
- Title: `Dependency Feedback Loop And Orchestration Board`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](./Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-083: Desktop Agents Panel And Focus Navigation](./Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `multi-agent, orchestrator, feedback-loop, dag-board, desktop, go, react`

## AI Quick View

### Summary

- Add dependency edges + a message bus to the `AgentOrchestrator` so a reviewer waits for a coder's diff, returns feedback, and the coder iterates to a round cap.
- Define bus message kinds (`ready-for-review`, `changes-requested`, `approved`, `rejected`, handoff) and the gate transitions that drive the loop.
- Build the Graph/DAG orchestration board in the desktop app: agents as nodes, dependencies/feedback as edges, round counter, run controls, and a live agent-bus log.

### Current Ask

- Implement the coder↔reviewer coordination logic + the Graph/DAG management view (mock frame 04), with pure-logic orchestrator tests and frontend tests.

### Key Decisions

- `T-1` The reviewer-gate is the agent-driven analogue of the human approval gate; `changes-requested` re-opens the coder's turn with the feedback as input.
- `T-2` The loop has a mandatory round cap and an explicit Stop control; on `approved`, hand off to the parent/main agent.
- `T-3` The management view is a Graph/DAG board (user-selected), not lanes.
- `T-4` Orchestrator transition logic is pure (no I/O), mirroring `workflow_state_machine.go`, for testability.

### Constraints

- Run GitNexus impact analysis before editing `agent_orchestrator.go` or the desktop store/board; warn on HIGH/CRITICAL.
- Reuse `artifact`/diff payloads for handoff; do not invent a new artifact channel.
- The loop must always terminate (round cap or stop).

### Open Questions

- Default round cap value and whether it is per-agent-definition configurable.

### Source Refs

- `CP-19` P-4, P-5; mock `output/cp19-multi-agent-mockup.html` frame 04; `workflow_orchestrator.go`, `workflow_state_machine.go`.

## 1. Goal

Coordinate parallel agents through dependencies and a feedback loop (canonical coder↔reviewer), and let the user observe/control it on a Graph/DAG board.

## 2. Parent Links

- coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `CP-19` P-4, P-5

## 3. Trigger

CP-19's core ask — a reviewer waiting on a coder, sending feedback, and the coder continuing — and the management view both require the dependency/bus logic and the board, which Tasks 081–083 do not provide.

## 4. Exact Change

- `T-1` Extend `AgentOrchestrator` with `dependsOn` edges, a message bus, and gate transitions for `ready-for-review` / `changes-requested` / `approved` / `rejected`.
- `T-2` Implement the bounded coder↔reviewer feedback loop with a round cap and stop.
- `T-3` New `OrchestrationBoard.tsx` (Graph/DAG): nodes, dependency/feedback edges, round counter, controls (resume, pause, inject feedback, add agent, stop all), live bus log.
- `T-4` Stream bus events to the board (reuse SSE).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/agent_orchestrator.go`, `interactive_handlers.go`, `apps/desktop-flowpilot/src/components/OrchestrationBoard.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `styles.css`
- modules: local-runner orchestration; desktop board + state
- routes: `GET /client/workflow-runs/{runId}/agent-bus` (or extend the events stream)
- tables: none (Phase 1)

## 6. Acceptance Check

- A coder + reviewer run in parallel; the reviewer stays blocked until `ready-for-review`, then reviews; `changes-requested` re-opens the coder with feedback; loop ends at `approved` or the round cap.
- The Graph/DAG board shows nodes, dependency/feedback edges, the current round, and a live bus log; controls (resume/pause/inject/stop) work.
- Orchestrator transition unit tests cover approve, changes-requested loop, and round-cap termination.

## 7. Out of Scope

- Supabase persistence and workflow-engine integration (Task-085); the lanes layout.

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
