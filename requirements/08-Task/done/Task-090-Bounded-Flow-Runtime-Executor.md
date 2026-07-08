# Task-090: Bounded Flow Runtime Executor

## Metadata

- Document ID: `Task-090`
- Title: `Bounded Flow Runtime Executor (Hub-Only Spawn / Join / Route / Reinvoke)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [Task-089: Generic Flow Vocabulary And flow_control Handler](./Task-089-Generic-Flow-Vocabulary-And-Flow-Control-Handler.md), [Task-084: Dependency Feedback Loop And Orchestration Board](../done/Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md)
- Replaces: `None`
- Tags: `multi-agent, flow-engine, executor, hub, bounded-loop, go`

## AI Quick View

### Summary

- Build the **domain-free executor**: `spawnNode` (inline on hub vs delegate to a child), `joinSatisfied` (all/any/quorum barrier), `route` (fire out-edges; bounded back-edges), and `applyFlowControl` (continue/done/escalate transitions) — driven by Task-089's vocabulary.
- Generalize `AgentLoopState` (add `OpenIssues`, `Mode`, `ActiveNode`, `ExtendCount`; rename `RoundCap`→`Cap`), add `TurnBridge.SubmitFlowControl`, and the `POST .../flow-control` + `.../agent-loop/extend-cap` routes.
- This is the synchronous core (works with `wait=true` spawns, no auto-reinvoke). The review loop is just configuration over it (Task-091).

### Current Ask

- Implement the executor + loop state + bridge + HTTP routes with bounded back-edges and bounded extend, hub-only coordination, and a domain-free guard — with pure-logic unit tests and contract tests.

### Key Decisions

- `T-1` The executor never names a role. Transitions are driven only by `flow_control` status + edge/policy data (CP-36 `P-1`/**G4**).
- `T-2` A back-edge increments the round and is **rejected at `round >= cap`** → `Status="blocked"`, `NextAction="awaiting_user"`. `reinvoke` restarts the target node with feedback; `once` respawns fresh (CP-36 `P-6`).
- `T-3` `run: inline` executes the node on the hub's own turn (no child); `delegate` spawns via the existing path (CP-36 **G2**).
- `T-4` Bounded extend: `extendCap` raises `Cap` by `ExtendBy` and increments `ExtendCount`, rejected once `ExtendCount >= ExtendMax` (ceiling = initial cap + 4) (SD-18 `D-9`).

### Constraints

- Run GitNexus impact analysis before editing `agent_orchestrator.go`, `interactive_service.go`, `provider_registry.go`, `provider_event.go`, `interactive_handlers.go`; warn on HIGH/CRITICAL.
- Additive only; do not alter `ProviderRuntimeAdapter` or the SSE union except additively.
- Every fake bridge implementing `TurnBridge` must be updated or the build breaks.
- **Prerequisite:** Task-084 orchestrator dependency/bus DTOs (CP-19) landed or frozen.

### Open Questions

- None. Inline-node turn reuses the parent's own turn path (`scheduleChildTurn(parentRunID,...)`).

### Source Refs

- CP-36 `P-2`, `G2`/`G4`/`G6`; SD-19 `D-1`/`D-6`, §7; SD-18 `D-3`…`D-9`, §7.
- Anchors: `agent_orchestrator.go` (`AgentLoopState`, `transition`, `advanceRound`); `interactive_service.go:394-571` (`dependenciesSatisfiedLocked`, `scheduleChildTurn`, `resumePendingLoopWork`, `takeQueuedFeedbackPrompt`), `:899-927` (restart path), `:1174` (`SpawnAgent` bridge); `provider_registry.go:25-32` (`TurnBridge`); `provider_event.go` (`AgentLoopState`); `interactive_handlers.go:41-48,860`.

## 1. Goal

A bounded, hub-only executor that runs any node/edge/policy graph: spawn (inline/delegate), join (all/any/quorum), route (forward + bounded back-edges), and apply `flow_control` to advance, terminate, or escalate — with loop state, a provider-facing bridge method, and HTTP control routes.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-2`
- tech design: [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) §7; [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) §7
- system spec: [SS-16](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) `AC-7`, `BR-6`
- specific upstream ids: CP-36 `P-2`; SD-19 `D-1`, `D-6`

## 3. Trigger

Task-089 gives the vocabulary but nothing runs it. The engine needs a bounded coordinator that spawns, joins, routes, and terminates flows generically, so the review loop (Task-091) and CP-41's Plan→Coding→Testing become config, not code.

## 4. Exact Change

- `T-1` Generalize `AgentLoopState` (`provider_event.go` Go + `contract.ts:123`): `Status` enum (`idle|running|waiting_join|synthesizing|blocked|done|stopped`), `Round`, `Cap` (was `RoundCap`), `GateReason`, + NEW `OpenIssues`, `Mode` (`keyword|explicit`), `ActiveNode`, `ExtendCount`. Keep JSON back-compat (additive; map old `roundCap` if present).
- `T-2` Executor methods in the orchestrator/service:
  - `spawnNode(parentRunID string, n FlowNode, deps []string)` — `Run=="inline"` → `scheduleChildTurn(parentRunID, parentStepID, nodePrompt)`; else `turnBridge.SpawnAgent` with `dependsOn=deps`.
  - `joinSatisfied(parentRunID, node)` — reuse `dependenciesSatisfiedLocked` logic for `all/any/quorum(n)` of the node's incoming edges.
  - `route(parentRunID, fromNode, status)` — fire out-edges whose `When==status`; a `Kind=="back"` edge calls `advanceRound` and is rejected when `round >= Cap`.
  - `applyFlowControl(parentRunID, in FlowControlInput) (FlowControlResult, error)`:
    - `done` → `transition(parentRunID,"done")`, `OpenIssues=0`, append hub handoff note, `NextAction="done"`, clear `autoOrchestrate`.
    - `continue` → `route`; if a back-edge would exceed `Cap` → `Status="blocked"`, `GateReason="cap reached with N open"`, `NextAction="awaiting_user"`; else re-enter the target (`reinvoke` restart via `:899-927`, `once` fresh spawn), `NextAction="looping"`.
    - `escalate` → `Status="blocked"`, `GateReason=in.Summary`, `NextAction="awaiting_user"`.
  - `extendCap(parentRunID)` — `+ExtendBy`, `ExtendCount++`, reject when `ExtendCount>=ExtendMax`; resume if was `blocked`.
- `T-3` `TurnBridge.SubmitFlowControl(in FlowControlInput) (FlowControlResult, error)` (`provider_registry.go:25-32`); implement `(*turnBridge).SubmitFlowControl` near `:1174` → `s.applyFlowControl(b.runID, in)`. Update every fake bridge implementing `TurnBridge`.
- `T-4` Routes (`interactive_handlers.go:41-48`, mirror `handleInjectAgentFeedback:860`): `POST /client/workflow-runs/{runId}/flow-control` → `handleSubmitFlowControl`; `POST /client/workflow-runs/{runId}/agent-loop/extend-cap` → `handleExtendCap` (body `{}`; resume if blocked).
- `T-5` `emitAgentGraph(parentRunID, snapshot)` on each transition so the board updates.

## 5. Touched Areas

- files: `agent_orchestrator.go`, `interactive_service.go`, `provider_registry.go`, `provider_event.go`, `interactive_handlers.go`, `contract.ts`; tests `agent_orchestrator_test.go`, `interactive_service_test.go`, every fake-bridge test (`codex_appserver_test.go`, `codex_resume_process_test.go`, `claude_adapter_test.go`, `fake_provider_adapter.go`).
- modules: local-runner flow engine + interactive service.
- routes: `POST .../flow-control`, `POST .../agent-loop/extend-cap`.
- tables: none.

## 6. Acceptance Check (Definition of Done)

- [ ] `applyFlowControl(done)` terminates the loop, zeroes `OpenIssues`, appends a hub handoff note, and clears `autoOrchestrate`.
- [ ] `applyFlowControl(continue)` with a forward edge fires the next node; with a back-edge under cap increments the round and re-enters the target.
- [ ] `reinvoke` re-entry **restarts the existing run** with the feedback prompt; `once` re-entry **spawns a fresh** node.
- [ ] A back-edge at `round >= Cap` sets `Status="blocked"`, `NextAction="awaiting_user"`, and does **not** re-enter the node.
- [ ] `applyFlowControl(escalate)` sets `blocked` + `awaiting_user`.
- [ ] `joinSatisfied` triggers correctly for `all`, `any`, and `quorum(n)` with children completing **out of order**.
- [ ] `spawnNode` with `run:inline` runs on the hub turn (no child spawned); `run:delegate` spawns a child with the right `dependsOn`.
- [ ] `extendCap` raises `Cap` by `ExtendBy`, increments `ExtendCount`, resumes from `blocked`, and is **rejected** once `ExtendCount>=ExtendMax`.
- [ ] **Domain-free guard test:** a test/grep asserts the executor source contains no `coder`/`reviewer`/`approved`/`changes_requested` literals.
- [ ] `TurnBridge.SubmitFlowControl` is implemented by the real bridge and every fake; the runner builds.
- [ ] `POST .../flow-control` and `POST .../agent-loop/extend-cap` round-trip and return the updated `FlowControlResult`/snapshot (HTTP contract test).
- [ ] Additive `AgentLoopState` JSON serializes with the new fields; old `roundCap` payloads still decode.

## 7. Out of Scope

- The review tool + provider registration + review config (Task-091); persistence/resume of loop state (Task-085); the consolidated join note (Task-092); auto-reinvocation (Task-093); skill/agent (Task-094); board UI (Task-095).

## 8. Completion Notes

- result: Implemented 2026-06-29. 9/9 new tests pass; all pre-existing orchestrator+spawn tests green; build clean. Added: `AgentLoopState{Cap,OpenIssues,Mode,ActiveNode,ExtendCount}`, `TurnBridge.SubmitFlowControl`, `turnBridge.SubmitFlowControl`, `applyFlowControl`, `extendCap`, `effectiveCap`, `handleSubmitFlowControl`, `handleExtendCap`, routes `POST .../flow-control` + `.../agent-loop/extend-cap`. `AgentLoopState` updated in `contract.ts`. All four fake bridges updated.
- follow-ups: Task-091 wires `submit_review_outcome` face onto this engine; Task-093 adds `autoOrchestrate` flag cleared on `done`.
- upstream docs updated: Task-090 status → done.
