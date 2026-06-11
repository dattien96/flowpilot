# Task-030: Workflow-Level YOLO Indicators And Runtime Simplification

## Metadata

- Document ID: `Task-030`
- Title: `Workflow-Level YOLO Indicators And Runtime Simplification`
- Phase: `task`
- Status: `todo`
- Owner: `Antigravity`
- Reviewers: `User`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`, `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- Child Documents: `none`
- Related Documents: `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`, `requirements/08-Task/done/Task-029-Workflow-Runs-Detail-Page-UX-Refinements.md`, `requirements/09-BugFix/done/BUG-036-Workflow-Run-Yolo-State-Drifts-From-Definition.md`, `requirements/09-BugFix/done/BUG-038-Single-Step-Yolo-Config-Is-Dropped-At-Runtime.md`, `requirements/09-BugFix/done/BUG-040-Admin-Web-Single-Step-Launch-Drops-Step-Yolo-Column.md`, `change-audit/CA-044-fix-workflow-run-yolo-default-drift.md`, `change-audit/CA-046-fix-single-step-yolo-config-drift.md`, `change-audit/CA-048-fix-admin-web-single-step-yolo-select.md`
- Replaces: `none`
- Tags: `ui, workflow-runs, yolo-mode, workflow-engine, refactor`

## AI Quick View

### Summary

- Task-030 simplifies YOLO back to one workflow-level source of truth for both UI and runtime behavior.
- Run detail UI should show execution-time workflow YOLO only, without step override or inherited-state explanations.
- Runtime start, approval checks, and resume logic should stop reading `step_definitions.yolo_mode` and `workflow_steps.yolo_mode`.
- Default YOLO remains `false` whenever no workflow-level value is available.

### Current Ask

- Update this task so it becomes the new source of truth for workflow-only YOLO behavior, covering both the run detail UI changes and the runtime refactor.

### Key Decisions

- `T-1` Use workflow-level YOLO only for execution-time behavior and runtime UI.
- `T-2` Keep `workflow_runs.yolo_mode` as the execution-time value shown in run details.
- `T-3` Do not resolve or display step-level YOLO override state in the new UI.
- `T-4` Remove runtime YOLO checks that read `workflow_steps.yolo_mode` or `step_definitions.yolo_mode`.
- `T-5` Treat Task-028 step-level YOLO precedence as historical and superseded by this task.

### Constraints

- Preserve the current CP-29 approval meaning: `false` requires approval and `true` allows uninterrupted auto-approved MCP execution.
- Display the execution-time YOLO value stored in `workflow_runs.yolo_mode`; do not recompute old runs from the current template state.
- Default to `false` when no workflow-level YOLO value exists.

### Open Questions

- None.

### Source Refs

- `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`
- `requirements/09-BugFix/done/BUG-036-Workflow-Run-Yolo-State-Drifts-From-Definition.md`
- `requirements/09-BugFix/done/BUG-038-Single-Step-Yolo-Config-Is-Dropped-At-Runtime.md`
- `requirements/09-BugFix/done/BUG-040-Admin-Web-Single-Step-Launch-Drops-Step-Yolo-Column.md`
- `change-audit/CA-044-fix-workflow-run-yolo-default-drift.md`
- `change-audit/CA-046-fix-single-step-yolo-config-drift.md`
- `change-audit/CA-048-fix-admin-web-single-step-yolo-select.md`
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`
- `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `supabase/functions/_shared/workflow-engine-runtime.ts`
- `supabase/functions/_shared/workflow-engine-state-machine.ts`

## 1. Goal

Simplify YOLO to a workflow-level execution rule only, so the workflow run details page and runtime engine both read one source of truth and stop exposing step-level override semantics.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`, `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- specific upstream ids: `Task-028`, `Task-029`, `BUG-036`, `BUG-038`, `BUG-040`

## 3. Trigger

The current `step > workflow` YOLO model is harder to reason about than the product needs. It forces the runtime, replay logic, and run-detail UI to carry step-level resolution rules, and it makes operator-facing indicators harder to follow. The project now wants to simplify YOLO so users see one workflow-level state and the runner enforces one workflow-level approval rule, with `false` as the default.

## 4. Exact Change

- `T-1` Domain and gateway cleanup:
  - Keep `WorkflowRun.yoloMode` mapped from `workflow_runs.yolo_mode`.
  - Stop requiring run-detail rendering to load or interpret `WorkflowStep.yoloMode` or `WorkflowDefinitionStep.yoloMode`.
  - Keep historical database columns readable only where needed for backward compatibility or migrations, not for new runtime/UI behavior.
- `T-2` Workflow run detail UI (`apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`):
  - Keep the header YOLO indicator based on the persisted execution-time run value.
  - If YOLO badges are shown in the sidebar list or step workspace, they must mirror workflow-level run YOLO only.
  - Remove step override wording such as `Step Override`, `Inherited`, or any step-specific effective YOLO explanation.
- `T-3` Runtime execution simplification (`apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`, `supabase/functions/_shared/workflow-engine-runtime.ts`, `supabase/functions/_shared/workflow-engine-state-machine.ts`):
  - Remove step-level YOLO precedence logic from execution, approval gating, and state progression.
  - Use workflow/run-level YOLO only when deciding whether MCP work pauses for approval or auto-approves.
  - Keep the existing CP-29 semantic mapping intact: `false` pauses for approval, `true` runs through.
- `T-4` Single-step launch simplification:
  - Stop copying `step_definitions.yolo_mode` into runtime-generated `workflows` or `workflow_steps` rows for new behavior.
  - Default single-step launches to workflow-level `false` unless a future explicit workflow-level source is introduced for that launch path.
- `T-5` Documentation cleanup:
  - Mark Task-028 as superseded for current YOLO behavior.
  - Mark the step-level YOLO bug and audit documents as historical fixes under the older rule, and point future readers to Task-030 for the current contract.

## 5. Touched Areas

- files:
  - `apps/admin-web/src/domain/model/entity/workflow.ts`
  - `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`
  - `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
  - `supabase/functions/_shared/workflow-engine-runtime.ts`
  - `supabase/functions/_shared/workflow-engine-state-machine.ts`
- modules: `admin-web`, `workflow-engine`, `supabase edge runtime`
- routes: `/workflow-runs/$runId`
- tables: `workflow_runs`, `workflows`, `workflow_steps`, `step_definitions`

## 6. Acceptance Check

- The top header YOLO badge accurately displays the execution-time YOLO mode (`ENABLED` or `DISABLED`) stored in `workflow_runs.yolo_mode`.
- Toggling current workflow or step-level YOLO settings after execution does not affect the loaded YOLO status of previous runs.
- Sidebar step rows and the step details workspace, if they display YOLO, reflect workflow-level run YOLO only and do not show override or inherited wording.
- Runtime start, approval gating, and resume logic use workflow/run YOLO only, with `false` as the default when no workflow-level YOLO value is present.

## 7. Out of Scope

- Toggling YOLO mode of a past execution run.
- Removing or migrating stored `step_definitions.yolo_mode` and `workflow_steps.yolo_mode` database columns.
- Hiding or deleting step-level YOLO controls outside the run-detail/runtime surfaces unless a separate task requests that cleanup.

## 8. Completion Notes

- result: Planned.
- follow-ups: Implement the workflow-only YOLO UI and runtime refactor, then review whether builder-time step YOLO controls should be removed in a separate task.
- upstream docs updated: `Task-028`, `BUG-036`, `BUG-038`, `BUG-040`, `CA-044`, `CA-046`, and `CA-048` should carry historical-rule notices that point readers to Task-030.
