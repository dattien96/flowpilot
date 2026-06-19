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
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-083: Desktop Agents Panel And Focus Navigation](./Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `multi-agent, orchestrator, feedback-loop, dag-board, desktop, go, react`

## AI Quick View

### Summary

- Add dependency edges + a message bus to the `AgentOrchestrator` so a reviewer waits for a coder's diff, returns feedback, and the coder iterates to a round cap.
- Define graph/bus DTOs and message kinds (`ready-for-review`, `changes-requested`, `approved`, `rejected`, `handoff`, `user-feedback`) plus the gate transitions that drive the loop.
- Build the Graph/DAG orchestration board in the desktop app: agents as nodes, dependencies/feedback as edges, round counter, run controls, and a live agent-bus log.

### Current Ask

- Implement the coder↔reviewer coordination logic + the Graph/DAG management view (redesign Frame 04), with pure-logic orchestrator tests, HTTP/contract tests, and frontend tests.

### Key Decisions

- `T-1` The reviewer-gate is the agent-driven analogue of the human approval gate; `changes-requested` re-opens the coder's turn with the feedback as input.
- `T-2` The loop has a mandatory round cap of 3 by default and an explicit Stop control; on `approved`, hand off to the parent/main agent.
- `T-3` The management view is a Graph/DAG board (user-selected), not lanes.
- `T-4` Orchestrator transition logic is pure (no I/O), mirroring `workflow_state_machine.go`, for testability.
- `T-5` Phase 1 bus/graph state is in-memory but replayable to the board through the existing parent-run SSE stream using additive event variants; durable `agent_messages` persistence is deferred to Task-085.
- `T-6` Busy-child `@mention` / injected feedback is queued as an agent-bus message for the next idle or blocked transition. It must not interrupt an active provider turn unless the user explicitly presses Stop.

### Constraints

- Run GitNexus impact analysis before editing `agent_orchestrator.go` or the desktop store/board; warn on HIGH/CRITICAL.
- Reuse `artifact`/diff payloads for handoff; do not invent a new artifact channel.
- The loop must always terminate (round cap or stop).
- Do not bypass existing child approval/question gates. Approval/question UI remains owned by the child run's normal stream and must stay interactive with YOLO=false.
- Do not add Supabase tables or cross-PC durable bus persistence in this task.

### Open Questions

- Whether the round cap should become per-agent-definition configurable after Phase 1. Phase 1 uses a default cap of 3 with an optional per-run override from the board.

### Source Refs

- `CP-19` P-4, P-5; Task-082 `AgentOrchestrator`, `AgentRunSummary`, `SpawnAgentInput`; Task-083 panel/focus store shape; mock `output/cp19-frames-02-04-redesign.html` (Frame 04 Graph/DAG orchestration board); `workflow_orchestrator.go`, `workflow_state_machine.go`.

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

- `T-1` Extend `AgentOrchestrator` with dependency/feedback edges, an in-memory bus, and pure transition functions for `ready-for-review` / `changes-requested` / `approved` / `rejected` / `handoff` / `user-feedback`.
- `T-2` Add contract DTOs in Go + TypeScript: `AgentDependencyEdge`, `AgentBusMessage`, `AgentLoopState`, and `AgentGraphSnapshot`. The snapshot includes `parentRunId`, current `AgentRunSummary[]`, edges, bus messages, current round, round cap, loop status, and active gate reason.
- `T-3` Implement the bounded coder↔reviewer feedback loop with default round cap 3, optional per-run override, queued feedback for busy children, and stop. `changes-requested` starts the coder's next normal child turn with the review text; it must not mutate provider transcripts directly.
- `T-4` Surface graph and bus updates through the existing parent-run SSE stream with additive event variants (for example `agent_graph_updated` and `agent_bus_message`) plus snapshot/history HTTP reads for first render and reconnect. Keep `streamRun(runId, afterSeq)` semantics intact.
- `T-5` New `OrchestrationBoard.tsx` (Graph/DAG): nodes, dependency/feedback edges, round counter, loop status, controls (resume loop, pause loop, inject feedback, add agent, stop all), live bus log, and affordances to focus an agent via Task-083.
- `T-6` Wire board controls to explicit runner methods. Pause/resume are loop-level soft gates only; they prevent or allow the next orchestrated turn and do not attempt to suspend an already-running provider stream. Stop uses existing child interrupt semantics where possible and marks the loop stopped.
- `T-7` Extend Task-083 store/client contract and mock client to hold `agentGraphSnapshot`, bus messages, `refreshAgentGraph()`, `pauseAgentLoop()`, `resumeAgentLoop()`, `injectAgentFeedback()`, and `stopAgentLoop()`.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/agent_orchestrator.go`, `provider_event.go`, `interactive_service.go`, `interactive_handlers.go`, `agent_orchestrator_test.go`, `apps/desktop-flowpilot/src/components/OrchestrationBoard.tsx`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `types/contract.ts`, `styles.css`
- modules: local-runner orchestration; desktop board + state
- routes: `GET /client/workflow-runs/{runId}/agent-graph`, `GET /client/workflow-runs/{runId}/agent-bus`, `POST /client/workflow-runs/{runId}/agent-loop/pause`, `POST /client/workflow-runs/{runId}/agent-loop/resume`, `POST /client/workflow-runs/{runId}/agent-loop/feedback`, `POST /client/workflow-runs/{runId}/agent-loop/stop`; live updates reuse the existing `GET /client/workflow-runs/{runId}/events/stream`
- tables: none (Phase 1)

## 6. Acceptance Check

- A coder + reviewer run in parallel; the reviewer stays blocked until `ready-for-review`, then reviews; `changes-requested` re-opens the coder with feedback; loop ends at `approved` or the round cap.
- The Graph/DAG board shows nodes, dependency/feedback edges, the current round/cap, loop status, active gate reason, and a live bus log; controls (resume/pause/inject/add agent/stop) work against the runner contract and mock client.
- Existing approval/question cards still appear for child runs with YOLO=false; the board may link to/focus the gated child but must not auto-approve, auto-answer, or hide the gate.
- `@coder ...` or board-injected feedback targeting a busy child queues a `user-feedback` bus message; it is consumed on the next safe turn and does not interrupt the provider unless Stop is pressed.
- Initial board render works from `AgentGraphSnapshot`; reconnect/replay works from the parent run's existing SSE `afterSeq` cursor.
- Orchestrator transition unit tests cover approve, rejected, changes-requested loop, queued feedback, pause/resume gate, explicit stop, child approval/question waiting state, and round-cap termination.
- HTTP/contract tests cover graph snapshot, bus history, loop control routes, and additive event DTO serialization. Frontend tests cover board rendering, live bus updates, control actions, focus affordance, and no-child/single-agent fallback.

## 7. Out of Scope

- Supabase persistence and workflow-engine integration (Task-085); the lanes layout; durable `agent_messages`; suspending an already-running provider stream; automatic approval/question decisions.

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
