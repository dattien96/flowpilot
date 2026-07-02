# BUG-155: Flow Mode Sidebar Shows Generic Step Label, Not Node Identity

## Metadata

- Document ID: `BUG-155`
- Title: `Flow Mode Sidebar Shows Generic Step Label, Not Node Identity`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`, `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-153-Flow-Mode-Right-Sidebar-Missing-Step-Execution-State.md`, `supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql`
- Replaces: `none`
- Tags: `agent-flow-engine, flow-gate, ui, workflow-steps-runtime, cp-42`

## AI Quick View

### Summary

- The "Workflow Steps" runtime sidebar (added by `BUG-153`) shows every `agent.delegate` node in a Review-Loop-shaped flow as the identical label "Flow: Agent Delegate", and every `hub.inline` node as "Flow: Hub Inline" — the user cannot tell the coder step apart from either reviewer step.
- CP-42 mirrors built-in agentpack flows into `workflows`/`workflow_steps` rows and gives each flow node its own real identity via `workflow_steps.node_id` ("coder", "reviewer_correctness", "reviewer_security", "synthesis") and `behavior_id`, while `step_type` stays a small set of shared, generic dispatch categories (`flow-agent-delegate`, `flow-hub-inline`, ...) seeded with a generic display name — by design, per the migration's own comment ("These rows are generic dispatch categories, not human-authored step definitions").
- The sidebar's data path (`SupabaseWorkflowStore.LoadRunSteps` → `workflowStepRuntimeView` → `WorkflowStepRuntimeDTO` → `WorkflowStepRuntimePanel.stepLabel()`) only ever read/returned `step_type`'s joined `step_definitions.name` — the generic dispatch-category label — and never selected `node_id`, so the real per-node identity was dropped before it ever reached the UI.
- The user also asked for the sidebar to show the step's model, provider, delegated agent file, and yolo posture, none of which were exposed at all.

### Current Ask

- Show the actual step/node name in the sidebar (not the shared generic dispatch-category label), plus the step's provider, model, delegated agent reference, and yolo-mode default.

### Key Decisions

- `F-1` Do not repurpose `RuntimeWorkflowStep.StepType`/`workflowStepRuntimeView.StepType` for display — other runtime code (`isCodingStepType`/`isPlanStepType` classification, per `BUG-NOTE-CP42 #7`) depends on `StepType` staying the literal dispatch-category/step-type key. Add a separate `NodeID` field end-to-end instead, mirroring the DB's own `node_id` vs `step_type` distinction (`20260701090000_add_flow_engine_attrs_to_workflows.sql`).
- `F-2` Source provider/model/agent/yolo from the step's own config, not from any execution-time resolution: `workflow_steps.provider_override` / `model_override` / `agent_ref` (the "agent file" the node delegates to) and `step_definitions.yolo_mode` (the step type's yolo default) are exactly the columns the rest of the app already treats as that step's configured values.
- `F-3` Desktop UI: reuse the existing `pill-prov prov-*` / `ac-model` badge classes already used for the same kind of provider/model display in `AgentsPanel.tsx`, for visual consistency rather than inventing new styling.

### Constraints

- `stepLabel()` must still fall back gracefully (`nodeId || stepType || stepId`) for classic, non-flow-engine workflow steps that have no `node_id` at all — this bug must not regress the plain workflow-step sidebar case `BUG-153` already covers.
- No change to `RuntimeWorkflowStep.StepType` semantics or to any classification logic that reads it.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/supabase_workflow_store.go:47-116` (`dbStep` / `LoadRunSteps` — select never carried `node_id`/`agent_ref`/`provider_override`/`model_override`/`yolo_mode`)
- `apps/local-runner/internal/runner/interactive_handlers.go:875-926` (`workflowStepRuntimeView` / `workflowStepsRuntime` — DTO never exposed these fields)
- `apps/desktop-flowpilot/src/components/WorkflowStepRuntimePanel.tsx:29-31` (`stepLabel()` only ever returned `stepType || stepId`)
- `supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql:53-102` (`node_id` vs `step_type` design intent; generic `flow-*` seed rows and their "Flow: ..." display names)

## 1. Issue Summary

In a Flow Mode run of a Review-Loop-shaped flow, the "Workflow Steps" runtime sidebar lists 4 steps as:

```
1. Flow: Agent Delegate   running
2. Flow: Agent Delegate   pending
3. Flow: Agent Delegate   pending
4. Flow: Hub Inline       pending
```

All three `agent.delegate` nodes (coder, reviewer_correctness, reviewer_security) render the identical generic label, so the user cannot tell which node is which. The sidebar also shows no model, provider, delegated-agent, or yolo information for any step.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, Flow Mode run of a built-in (or user-cloned) flow whose nodes share a behavior, e.g. Review Loop (`coder`, `reviewer_correctness`, `reviewer_security` all on `agent.delegate`; `synthesis` on `hub.inline`).
- reproduction steps:
  1. Start a Flow Mode run using the Review Loop flow (or any flow with more than one node on the same behavior).
  2. Open the "Workflow Steps" right-sidebar (`WorkflowStepRuntimePanel`).
  3. Observe that every `agent.delegate` node shows the same "Flow: Agent Delegate" label, and no model/provider/agent/yolo data is shown for any step.
- frequency: deterministic for any flow with 2+ nodes sharing a behavior — always for built-in Review Loop, which has 3.

## 4. Expected vs Actual

- expected: Each step shows its own name (the flow node's identity — e.g. "coder", "reviewer_correctness", "reviewer_security", "synthesis") plus its provider, model, delegated agent reference, and yolo-mode default.
- actual: Every node sharing a behavior renders the identical generic dispatch-category label ("Flow: Agent Delegate" / "Flow: Hub Inline"), and no provider/model/agent/yolo data is shown at all.

## 5. Impact

- users affected: anyone running a Flow Mode flow with more than one node on the same behavior (every built-in Review-Loop-shaped flow).
- workflows affected: Flow Mode sidebar visibility only — execution itself is unaffected, this is a read/display-only gap.
- severity: medium — no data corruption or execution failure, but the sidebar that `BUG-153` added specifically to let users "see at a glance which step is active" fails at exactly that for any multi-node-same-behavior flow.

## 6. Root Cause

- hypothesis: display formatting issue in the desktop component.
- confirmed cause: the per-node identity (`workflow_steps.node_id`) and per-node config (`agent_ref`, `provider_override`, `model_override`, and the step type's `yolo_mode`) were never selected by the Go backend at all. `SupabaseWorkflowStore.LoadRunSteps`'s PostgREST select only embedded `workflow_steps(requires_approval,behavior_id)`, so `RuntimeWorkflowStep` (and therefore the `/client/workflow-runs/{runId}/steps-runtime` DTO the desktop reads) only ever carried `step_type`'s joined `step_definitions.name` — which, for CP-42 flow-engine nodes, is one of a handful of generic dispatch-category display names ("Flow: Agent Delegate", "Flow: Hub Inline", ...) shared by every node running that behavior, by design (per the `20260701090000_add_flow_engine_attrs_to_workflows.sql` migration comment: "These rows are generic dispatch categories, not human-authored step definitions — a node's specific behavior/agent binding always comes from its own `behavior_id`/`agent_ref` columns, never from this row's fields"). `WorkflowStepRuntimePanel.stepLabel()` had nothing else to fall back to but `stepType || stepId`, both of which are also generic/opaque for these nodes.
- evidence:
  - `supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql:53-102` — introduces `node_id`/`behavior_id`/`agent_ref` on `workflow_steps` specifically so a node's real identity is decoupled from `step_type`, and seeds the shared generic `step_definitions` rows whose `name` is literally "Flow: Agent Delegate" / "Flow: Hub Inline" / etc.
  - `apps/local-runner/internal/runner/supabase_workflow_store.go` (pre-fix) — `dbStep.WorkflowSteps` only had `RequiresApproval`/`BehaviorID`; the select string never requested `node_id`, `agent_ref`, `provider_override`, or `model_override`.
  - `apps/local-runner/internal/runner/interactive_handlers.go` (pre-fix) — `workflowStepRuntimeView` had no field for any of the above.
  - `apps/desktop-flowpilot/src/components/WorkflowStepRuntimePanel.tsx` (pre-fix) — `stepLabel()` = `step.stepType || step.stepId`, with no per-node identity to prefer.

## 7. Fix Strategy

- `F-1` Extend `RuntimeWorkflowStep` (`workflow_state_machine.go`) with `NodeID`, `AgentRef`, `Provider`, `Model`, `YoloMode`.
- `F-2` Extend `SupabaseWorkflowStore.LoadRunSteps`'s select to embed `workflow_steps(...,node_id,agent_ref,provider_override,model_override,step_definitions(yolo_mode))` and map the new columns onto `RuntimeWorkflowStep` (`supabase_workflow_store.go`).
- `F-3` Extend the `workflowStepRuntimeView` DTO and `workflowStepsRuntime` mapping (`interactive_handlers.go`) to round-trip `nodeId`/`agentRef`/`provider`/`model`/`yoloMode` over `GET /client/workflow-runs/{runId}/steps-runtime`.
- `F-4` Extend the desktop `WorkflowStepRuntimeDTO` contract (`contract.ts`) with the same fields.
- `F-5` `WorkflowStepRuntimePanel.stepLabel()` now prefers `nodeId` over `stepType`/`stepId`; the step card's meta row now renders a `pill-prov prov-*` provider badge, an `ac-model` model chip, the `agentRef` (labelled "agent: ..."), and a "yolo" tag when `yoloMode` is true — reusing the same badge classes `AgentsPanel.tsx` already uses for agent runs, for visual consistency.

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` — passes.
- `V-2` `go test ./internal/runner/... -run 'TestSupabaseStoreLoadRunStepsShaping|TestWorkflowStepsRuntime'` — passes; `TestSupabaseStoreLoadRunStepsShaping` was updated to assert the new select clause and to decode `node_id`/`agent_ref`/`provider_override`/`model_override`/`step_definitions.yolo_mode` into the matching `RuntimeWorkflowStep` fields.
- `V-3` `npm run typecheck` in `apps/desktop-flowpilot` — no new errors attributable to `WorkflowStepRuntimePanel.tsx` or `contract.ts` (the run surfaced ~31 pre-existing `pendingApproval`/`pendingApprovals` errors in unrelated, already-modified files — `ApprovalCard.tsx`, `ChatInput.tsx`, `state/store.ts`, `state/store.test.ts`, `state/timelineReducer.test.ts` — none of which this fix touches; confirmed pre-existing by grepping the typecheck output for this fix's files, which returned no matches).
- `V-4` Not executed: a live manual repro (starting an actual Flow Mode Review Loop run and inspecting the rendered sidebar), because no running desktop app / local Supabase instance was available in this environment. This should be confirmed visually before merge.

## 9. Regression Guard

- tests: `TestSupabaseStoreLoadRunStepsShaping` now covers the full new select/decode path with a flow-engine-shaped row (`step_type: flow-agent-delegate`, `node_id: coder`, etc.), so a future accidental removal of any of these columns from the select will fail the test.
- alerts: none.
- audit checks: recorded in `change-audit/CA-191-flow-mode-sidebar-node-identity-and-config.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a display-fidelity fix on top of CP-42's already-documented `node_id`/`behavior_id`/`agent_ref` design; no business rule changed.
- notes left unchanged on purpose: the unrelated pre-existing `pendingApproval`/`pendingApprovals` typecheck errors found in `ApprovalCard.tsx`/`ChatInput.tsx`/`state/store.ts`/tests during `V-3` are left as-is — they predate and are unrelated to this fix and belong to whatever other in-progress change introduced them.
