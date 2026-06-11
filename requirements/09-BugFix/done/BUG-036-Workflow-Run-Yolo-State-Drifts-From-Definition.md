# BUG-036: Workflow Run YOLO State Drifts From Definition

## Metadata

- Document ID: `BUG-036`
- Title: `Workflow Run YOLO State Drifts From Definition`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- Child Documents: `none`
- Related Documents: `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`
- Replaces: `none`
- Tags: `workflow-engine, yolo, runtime, cp-29, regression`

## AI Quick View

### Summary

- Workflow definitions and workflow-attached steps can already store YOLO intent, including step overrides from Task-028.
- The local runtime path starts runs with the workflow default, but the Supabase start-run edge path still hardcoded `workflow_runs.yolo_mode = false`.
- That made runtime state look like a separate YOLO configuration instead of reflecting the saved workflow definition.
- The run detail header also labeled the indicator as plain `YOLO`, which reinforced the impression that it was an independent runtime switch.

### Current Ask

- Record the fix so workflow runs inherit the workflow-level YOLO default consistently and the runtime UI labels the badge as workflow-level state.

### Key Decisions

- `V-1` New workflow runs must initialize `workflow_runs.yolo_mode` from `workflows.yolo_mode`, not a hardcoded false value.
- `V-2` Runtime UI should label the displayed YOLO state as workflow-level state, because step overrides are resolved separately during execution.

### Constraints

- Keep Task-028 behavior where step execution still resolves effective YOLO as `workflow_steps.yolo_mode ?? workflows.yolo_mode`.
- Do not silently redefine `workflow_runs.yolo_mode` as per-step state in this fix.
- Keep CP-29 provider-session YOLO behavior driven by effective step execution state.

### Open Questions

- Should run detail eventually show both workflow default YOLO and the selected step's effective YOLO so mixed workflows are explicit?

### Source Refs

- `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`
- `supabase/functions/workflow-engine-start-run/index.ts`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`

## 1. Issue Summary

Workflow execution already resolves step-level YOLO correctly, but the runtime-facing run state could drift from the saved workflow definition because one start-run path always created `workflow_runs.yolo_mode` as `false`. That made the runtime badge and any run-level reads look detached from the workflow or step YOLO values configured in the builder.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- impacted system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`

## 3. Environment and Reproduction

- environment: FlowPilot workflow runtime using the Supabase `workflow-engine-start-run` entrypoint, especially CP-29 workflows with step-level Google Drive approval behavior
- reproduction steps:
  1. Save a workflow with workflow-level YOLO enabled, or save a workflow that relies on step-level YOLO overrides from Task-028.
  2. Start the workflow through the Supabase start-run edge path.
  3. Open the workflow run detail page and compare the displayed run YOLO state with the definition that was saved in the workflow builder.
  4. Inspect the start-run implementation and compare it with the local runtime implementation.
- frequency: deterministic whenever the start-run edge function is the code path that creates the workflow run row

## 4. Expected vs Actual

- expected: new workflow runs should inherit the workflow-level YOLO default from the saved workflow definition, and the runtime UI should make it clear that the badge reflects workflow-level state rather than an independent runtime-only setting
- actual: the Supabase start-run edge path inserted `workflow_runs.yolo_mode = false` regardless of the saved workflow definition, and the runtime badge was labeled generically as `YOLO`

## 5. Impact

- users affected: users running workflows that rely on YOLO defaults or comparing CP-29 runtime approval behavior against saved workflow configuration
- workflows affected: workflow run creation, run detail display, and operator understanding of CP-29 approval behavior
- severity: medium because step execution remained correct in the local runtime path, but runtime state and runtime messaging could contradict the saved workflow definition

## 6. Root Cause

- hypothesis: a legacy run-level YOLO initialization path was left behind after Task-028 introduced definition-driven step override semantics
- confirmed cause: `supabase/functions/workflow-engine-start-run/index.ts` hardcoded `workflow_runs.yolo_mode` to `false`, while `runWorkflowStartRuntime` already initialized the same field from `Boolean(workflow.yolo_mode)`; the run detail UI also labeled the value too generically
- evidence:
  - the Supabase start-run function inserted `yolo_mode: false`
  - the local runtime implementation inserted `yolo_mode: Boolean(workflow.yolo_mode)`
  - Task-028 explicitly states runtime execution should use step override precedence while keeping `workflow_runs.yolo_mode` as the workflow-level default

## 7. Fix Strategy

- `F-1` Initialize `workflow_runs.yolo_mode` from `Boolean(workflow.yolo_mode)` in the Supabase start-run function.
- `F-2` Rename the run detail badge label and aria text to `Workflow YOLO` so it is understood as workflow-level state, not a separate runtime control.

## 8. Validation

- `V-1` Code inspection confirms both workflow run creation paths now initialize `workflow_runs.yolo_mode` from the workflow definition default.
- `V-2` Run detail now labels the badge as workflow-level YOLO state instead of a generic standalone `YOLO` toggle.

## 9. Regression Guard

- tests:
  - existing workflow runtime tests were reviewed to confirm step-level execution still uses `workflow_steps.yolo_mode ?? workflows.yolo_mode`
  - no direct automated test currently covers the Supabase start-run edge function initialization path
- alerts:
  - none added
- audit checks:
  - keep all workflow run creation entrypoints aligned on `workflow_runs.yolo_mode`
  - keep run-level display text explicit when step-level overrides remain possible

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - none required for this fix because the implementation is being brought back into alignment with Task-028 and CP-29
- notes left unchanged on purpose:
  - Task-028 continues to treat `workflow_runs.yolo_mode` as the workflow-level default and effective per-step YOLO as a separate runtime resolution
