# CA-191: Flow Mode Sidebar Node Identity And Config

## Summary

Fixed `BUG-155`: the Flow Mode "Workflow Steps" runtime sidebar showed every node sharing a behavior (e.g. all three `agent.delegate` nodes in Review Loop) under the identical generic label "Flow: Agent Delegate", and exposed no provider/model/agent/yolo data per step.

## What Changed

- `apps/local-runner/internal/runner/workflow_state_machine.go`: added `NodeID`, `AgentRef`, `Provider`, `Model`, `YoloMode` to `RuntimeWorkflowStep`.
- `apps/local-runner/internal/runner/supabase_workflow_store.go`: `LoadRunSteps`'s PostgREST select now embeds `workflow_steps(...,node_id,agent_ref,provider_override,model_override,step_definitions(yolo_mode))` and maps them onto the new `RuntimeWorkflowStep` fields.
- `apps/local-runner/internal/runner/interactive_handlers.go`: `workflowStepRuntimeView` and `workflowStepsRuntime` now round-trip `nodeId`/`agentRef`/`provider`/`model`/`yoloMode` over `GET /client/workflow-runs/{runId}/steps-runtime`.
- `apps/desktop-flowpilot/src/types/contract.ts`: `WorkflowStepRuntimeDTO` gained the matching fields.
- `apps/desktop-flowpilot/src/components/WorkflowStepRuntimePanel.tsx`: `stepLabel()` now prefers `nodeId` over the shared generic `stepType`/`stepId`; the step card meta row renders provider/model/agent/yolo, reusing `AgentsPanel.tsx`'s existing `pill-prov prov-*` / `ac-model` badge classes.
- `apps/local-runner/internal/runner/phase5_test.go`: `TestSupabaseStoreLoadRunStepsShaping` updated to assert the new select clause and decode the new fields from a flow-engine-shaped row.

## Verification

- `go build ./...` in `apps/local-runner` — passes.
- `go test ./internal/runner/... -run 'TestSupabaseStoreLoadRunStepsShaping|TestWorkflowStepsRuntime'` — passes.
- `npm run typecheck` in `apps/desktop-flowpilot` — no errors attributable to the two changed TS/TSX files; ~31 pre-existing `pendingApproval`/`pendingApprovals` errors in unrelated already-modified files (`ApprovalCard.tsx`, `ChatInput.tsx`, `state/store.ts`, tests) are unrelated to this change (confirmed unaffected by this diff).
- Not executed: manual visual repro in a running desktop app against a live Supabase instance (none available in this environment).

## Notes

- `RuntimeWorkflowStep.StepType` and the DTO's `stepType` are deliberately left untouched — other runtime code classifies steps by `StepType`/`BehaviorID` (`BUG-NOTE-CP42 #7`), so the fix adds a separate `NodeID` identity field rather than repurposing `StepType`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-155
change_type: bugfix
summary: Surface flow node identity (node_id) and per-step provider/model/agent/yolo config in the workflow-step runtime sidebar instead of the shared generic step_type label
# --->8---
