# Task-089: Generic Flow Vocabulary And flow_control Handler

## Metadata

- Document ID: `Task-089`
- Title: `Generic Flow Vocabulary And flow_control Handler`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [Task-090: Bounded Flow Runtime Executor](./Task-090-Bounded-Flow-Runtime-Executor.md), [Task-091: Review-Loop Template](./Task-091-Review-Loop-Template.md)
- Replaces: `None`
- Tags: `multi-agent, flow-engine, generic, node-edge-policy, flow-control, go`

## AI Quick View

### Summary

- Define the **domain-free vocabulary** every flow compiles to: `FlowNode` (who + run/lifecycle/join), `FlowEdge` (conditional, forward/back), `FlowPolicy` (cap + bounded extend), and the one generic control signal `FlowControlInput{status: continue|done|escalate}`.
- Add `parseFlowControlInput` and a **declared-face registry** that maps a domain tool's statuses onto the three generic ones (e.g. `approved→done`, `changes_requested→continue`, `blocked→escalate`).
- Pure types + parsing + mapping only — **no behavior change**. The executor that consumes them is Task-090; the review face that declares onto them is Task-091.

### Current Ask

- Implement the engine vocabulary, the generic control parser, and the declared-face mapping table, with defaults and validation, and unit tests — without touching any runtime behavior.

### Key Decisions

- `T-1` The engine speaks only `continue|done|escalate`. Domain tools never reach the executor directly; they declare a mapping into these three (SD-19 `D-4`). This is the single seam that keeps the engine domain-free.
- `T-2` Edge `When` is matched against the **mapped generic status**, never a domain string (CP-36 **G5**). Load-time validation rejects more than one back-edge target per status (CP-36 **G6**).
- `T-3` Defaults applied on parse: `Run=delegate`, `Lifecycle=reinvoke`, `Join=all`, `Cap=3`, `OnCap=escalate`, `ExtendBy=2`, `ExtendMax=2`.

### Constraints

- Run GitNexus impact analysis before editing `agent_orchestrator.go`; warn on HIGH/CRITICAL.
- Additive, pure types/functions only; no change to spawn, loop, or provider behavior.
- No role/use-case strings (`coder`/`reviewer`/`approved`) in these types or the handler.

### Open Questions

- None. Quorum syntax fixed as `quorum(n)` parsed to an int threshold.

### Source Refs

- CP-36 `P-1`, `G3`/`G5`/`G6`; SD-19 `D-1`/`D-4`, §5/§6.
- Anchors: `agent_orchestrator.go:344` (`parseSpawnAgentInput` pattern).

## 1. Goal

Provide the data vocabulary (`FlowNode`/`FlowEdge`/`FlowPolicy`), the generic `FlowControlInput`/`FlowControlResult`, `parseFlowControlInput`, and a declared-face mapping registry — the contract Task-090's executor and Task-091's review face both build on.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-1`
- tech design: [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) `D-1`, `D-4`, §5/§6
- system spec: [SS-16](../../05-System-Specs/SS-16-Agent-Flow-Engine.md) `AC-3`, `BR-1`
- specific upstream ids: CP-36 `P-1`; SD-19 `D-4`

## 3. Trigger

The executor (Task-090) needs a stable, domain-free transition vocabulary, and the review face (Task-091) needs a generic target to map onto. Building these as pure data first keeps the engine free of role logic and makes both downstream tasks independently testable.

## 4. Exact Change

- `T-1` Add to `agent_orchestrator.go` (new "flow vocabulary" section):
  - `FlowNode{ID string; Agent string; Run string; Lifecycle string; Join string}`
  - `FlowEdge{From string; To string; When string; Kind string}`
  - `FlowPolicy{Cap int; OnCap string; ExtendBy int; ExtendMax int}`
  - `FlowControlInput{Status string; Summary string; Payload map[string]any}`
  - `FlowControlResult{Status string; Round int; Cap int; OpenIssues int; NextAction string}`
- `T-2` `parseFlowControlInput(args map[string]any) (FlowControlInput, error)` mirroring `parseSpawnAgentInput:344`; validate `Status ∈ {continue,done,escalate}`; carry arbitrary `Payload` opaquely.
- `T-3` `applyFlowNodeDefaults(*FlowNode)` / `applyFlowPolicyDefaults(*FlowPolicy)` setting the `T-3` defaults; `parseJoin(string) (mode string, n int)` parsing `all|any|quorum(n)`.
- `T-4` Declared-face registry: `type FlowControlFace struct{ Tool string; Map map[string]string }` + `resolveFaceStatus(face, domainStatus) (genericStatus string, ok bool)`. Provide a `reviewOutcomeFace()` constructor returning the `approved→done / changes_requested→continue / blocked→escalate` map (used by Task-091; defined here so the mapping table lives with the engine).
- `T-5` Load-time validator `validateFlowEdges([]FlowEdge) error` rejecting >1 back-edge (`Kind=="back"`) target per `When` status.

## 5. Touched Areas

- files: `agent_orchestrator.go`, `agent_orchestrator_test.go`.
- modules: local-runner orchestrator (types only).
- routes: none.
- tables: none.

## 6. Acceptance Check (Definition of Done)

- [ ] `parseFlowControlInput` accepts `continue|done|escalate` and rejects any other status with a clear error; `Payload` round-trips opaquely.
- [ ] `applyFlowNodeDefaults`/`applyFlowPolicyDefaults` produce `Run=delegate`, `Lifecycle=reinvoke`, `Join=all`, `Cap=3`, `OnCap=escalate`, `ExtendBy=2`, `ExtendMax=2` when fields are empty/zero.
- [ ] `parseJoin` returns `("all",0)`, `("any",0)`, and `("quorum",n)` for `quorum(n)`; rejects malformed quorum.
- [ ] `resolveFaceStatus(reviewOutcomeFace(), s)` maps `approved→done`, `changes_requested→continue`, `blocked→escalate`, and returns `ok=false` for unknown.
- [ ] `validateFlowEdges` rejects a node with two `back` edges firing on the same status; accepts multiple `forward` edges on one status.
- [ ] FlowNode/FlowEdge/FlowPolicy/FlowControlInput/FlowControlResult JSON marshal/unmarshal round-trip.
- [ ] **No role/use-case strings** appear in the new types or handler (review/grep check).
- [ ] The runner builds; no existing test changes behavior (pure-additive).

## 7. Out of Scope

- The executor and transitions (Task-090); the review tool + provider registration (Task-091); persistence (Task-085); the consolidated note (Task-092); auto-reinvoke (Task-093); skill/agent (Task-094); UI (Task-095).

## 8. Completion Notes

- result: Implemented 2026-06-29. All 14 new tests pass; all pre-existing orchestrator tests green. Build clean. Added to `agent_orchestrator.go`: `FlowNode`, `FlowEdge`, `FlowPolicy`, `FlowControlInput`, `FlowControlResult`, `FlowControlFace`, `parseFlowControlInput`, `applyFlowNodeDefaults`, `applyFlowPolicyDefaults`, `parseJoin`, `resolveFaceStatus`, `reviewOutcomeFace`, `validateFlowEdges`.
- follow-ups: Task-090 (executor consumes these types), Task-091 (registers `submit_review_outcome` face).
- upstream docs updated: Task-089 status → done.
