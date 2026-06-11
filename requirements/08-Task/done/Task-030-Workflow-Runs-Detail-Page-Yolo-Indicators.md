# Task-030: Workflow-Level YOLO Indicators And Live Policy Simplification

## Metadata

- Document ID: `Task-030`
- Title: `Workflow-Level YOLO Indicators And Live Policy Simplification`
- Phase: `task`
- Status: `done`
- Owner: `Antigravity`
- Reviewers: `User`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`, `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- Child Documents: `none`
- Related Documents: `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`, `requirements/08-Task/done/Task-029-Workflow-Runs-Detail-Page-UX-Refinements.md`, `requirements/08-Task/done/Task-031-Single-Step-Step-Definition-Yolo-Policy.md`, `requirements/09-BugFix/done/BUG-036-Workflow-Run-Yolo-State-Drifts-From-Definition.md`, `requirements/09-BugFix/done/BUG-038-Single-Step-Yolo-Config-Is-Dropped-At-Runtime.md`, `requirements/09-BugFix/done/BUG-040-Admin-Web-Single-Step-Launch-Drops-Step-Yolo-Column.md`, `change-audit/CA-044-fix-workflow-run-yolo-default-drift.md`, `change-audit/CA-046-fix-single-step-yolo-config-drift.md`, `change-audit/CA-048-fix-admin-web-single-step-yolo-select.md`, `change-audit/CA-051-single-step-step-definition-yolo-policy.md`
- Replaces: `none`
- Tags: `ui, workflow-runs, yolo-mode, workflow-engine, refactor`

## AI Quick View

### Summary

- Task-030 is the source of truth for workflow-definition runs: always use current `workflows.yolo_mode`.
- A workflow with exactly one child step is still a workflow-definition run and must use `workflows.yolo_mode`.
- Run detail UI should show the current effective YOLO policy, not a frozen run-time snapshot, and must not mutate YOLO.
- Task-031 defines the only accepted exception: direct single-step runs use current `step_definitions.yolo_mode`.

### Current Ask

- This task is now the active source of truth for workflow-only YOLO behavior across the run detail UI and runtime execution paths.

### Key Decisions

- `T-1` Use workflow-level YOLO only for workflow-definition execution behavior and runtime UI.
- `T-2` Use the current `workflows.yolo_mode` value when resuming or continuing a workflow-definition run, even if the run originally started under a different YOLO setting.
- `T-3` Do not resolve or display workflow child-step YOLO override state in the run UI.
- `T-4` Do not read `workflow_steps.yolo_mode`; that column must not be part of the current model.
- `T-5` Treat `workflow_runs.yolo_mode` as historical data only, not the source of truth for current continue/resume behavior.
- `T-6` Follow Task-031 for direct single-step runs, where `step_definitions.yolo_mode` is the SSOT.

### Constraints

- Preserve the current CP-29 approval meaning: `false` requires approval and `true` allows uninterrupted auto-approved MCP execution.
- Show and enforce the current workflow-level YOLO policy from `workflows.yolo_mode`, not the old persisted run snapshot.
- Show YOLO in run detail as read-only status; do not provide a run-page mutation control.
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

Simplify YOLO to a workflow-level live policy only, so the workflow run details page and runtime engine both read the current workflow definition and stop exposing step-level override semantics or frozen run-level YOLO snapshots.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`, `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- specific upstream ids: `Task-028`, `Task-029`, `BUG-036`, `BUG-038`, `BUG-040`

## 3. Trigger

The current `step > workflow` YOLO model is harder to reason about than the product needs. It forces the runtime, replay logic, and run-detail UI to carry step-level resolution rules, and it makes operator-facing indicators harder to follow. The project now wants to simplify YOLO so users see one workflow-level state and the runner enforces one workflow-level approval rule, with `false` as the default. When an operator changes workflow YOLO after an earlier run already exists, the next continue/resume action should use the current workflow setting rather than the older run snapshot.

## 4. Exact Change

- `T-1` Domain and gateway cleanup:
  - Keep `WorkflowRun.yoloMode` only if still needed for historical display or backward compatibility.
  - Stop requiring run-detail rendering to load or interpret `WorkflowStep.yoloMode` or `WorkflowDefinitionStep.yoloMode`.
  - Keep historical database columns readable only where needed for backward compatibility or migrations, not for new runtime/UI behavior.
- `T-2` Workflow run detail UI (`apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`):
  - Show the header YOLO indicator from the current workflow definition value, not from `workflow_runs.yolo_mode`.
  - If YOLO badges are shown in the sidebar list or step workspace, they must mirror workflow-level run YOLO only.
  - Remove step override wording such as `Step Override`, `Inherited`, or any step-specific effective YOLO explanation.
  - Do not expose any run-detail YOLO toggle or mutation action; the run page is display-only for YOLO.
- `T-3` Runtime execution simplification (`apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`, `supabase/functions/_shared/workflow-engine-runtime.ts`, `supabase/functions/_shared/workflow-engine-state-machine.ts`):
  - Remove step-level YOLO precedence logic from execution, approval gating, and state progression.
  - Use the current workflow-level YOLO only when deciding whether MCP work pauses for approval or auto-approves.
  - Keep the existing CP-29 semantic mapping intact: `false` pauses for approval, `true` runs through.
- `T-4` Continue/resume behavior:
  - When an existing run is resumed or continued, recalculate the effective YOLO value from the current `workflows.yolo_mode` row.
  - Do not let an older `workflow_runs.yolo_mode` value keep a run on outdated approval behavior after the workflow definition changed.
- `T-4` Single-step launch rule:
  - Superseded by Task-031: direct single-step launches use `step_definitions.yolo_mode` as their SSOT.
  - A workflow definition that contains one step is not a direct single-step launch and still uses `workflows.yolo_mode`.
- `T-5` Documentation cleanup:
  - Mark Task-028 as superseded for current YOLO behavior.
  - Mark the step-level YOLO bug and audit documents as historical fixes under the older rule, and point future readers to Task-030 for the current contract.
- `T-6` Schema and builder cleanup:
  - Drop the legacy `workflow_steps.yolo_mode` column with a forward SQL migration.
  - Keep `step_definitions.yolo_mode` only for Task-031 direct single-step launches.
  - Remove workflow child-step YOLO controls from workflow editor pages.

## 5. Touched Areas

- files:
  - `apps/admin-web/src/domain/model/entity/workflow.ts`
  - `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`
  - `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
  - `apps/admin-web/src/routes/_authenticated/workflow-steps/create.tsx`
  - `apps/admin-web/src/routes/_authenticated/workflow-steps/$stepType.tsx`
  - `apps/admin-web/src/routes/_authenticated/workflows/create.tsx`
  - `apps/admin-web/src/routes/_authenticated/workflows/$workflowId.tsx`
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
  - `supabase/functions/_shared/workflow-engine-runtime.ts`
  - `supabase/functions/_shared/workflow-engine-state-machine.ts`
  - `supabase/migrations/20260611103000_drop_step_level_yolo_columns.sql`
- modules: `admin-web`, `workflow-engine`, `supabase edge runtime`
- routes: `/workflow-runs/$runId`
- tables: `workflow_runs`, `workflows`, `workflow_steps`, `step_definitions`

## 6. Acceptance Check

- The top header YOLO badge displays the current workflow YOLO mode (`ENABLED` or `DISABLED`) from `workflows.yolo_mode`.
- If a workflow's YOLO setting changes after an earlier run already exists, continuing or resuming that workflow-definition run uses the new workflow YOLO value.
- The run detail page displays YOLO only and does not allow changing it.
- Sidebar step rows and the step details workspace, if they display YOLO, reflect current workflow-level YOLO only and do not show override or inherited wording.
- Runtime start, approval gating, and resume logic use current workflow YOLO only, with `false` as the default when no workflow-level YOLO value is present.

## 7. Out of Scope

- Toggling YOLO mode of a past execution run.
- Removing the workflow-level `workflows.yolo_mode` or historical `workflow_runs.yolo_mode` columns.
- Adding or preserving a run-detail YOLO mutation control.
- Defining direct single-step YOLO behavior beyond the Task-031 rule that direct single-step launches use `step_definitions.yolo_mode`.

## 8. Completion Notes

- result: Implemented workflow-only YOLO handling for the run detail UI, admin-web runtime flow, and Supabase shared approval-progress path.
- result: Dropped legacy workflow-step YOLO persistence with `supabase/migrations/20260611103000_drop_step_level_yolo_columns.sql` and removed builder-time child-step YOLO controls from workflow editor pages.
- follow-ups: `Task-031` restores `step_definitions.yolo_mode` only for direct single-step launches; normal workflow runs, including one-step workflows, remain workflow-level-only and `workflow_steps.yolo_mode` stays removed.
- follow-ups: Remove or disable any remaining legacy run-level YOLO mutation API/UI in a separate cleanup task.
- upstream docs updated: `Task-028`, `BUG-036`, `BUG-038`, `BUG-040`, `CA-044`, `CA-046`, and `CA-048` now point to Task-030 as the current YOLO contract.

## 9. Manual Test Steps - Passed

1. Prepare one workflow with at least one approval-gated step:
   - Open the workflow in the builder.
   - Confirm `workflows.yolo_mode = false`.
   - Make sure the workflow has at least one step that normally pauses for approval.

2. Verify the disabled-path run behavior:
   - Start a new run for that workflow.
   - Open `/workflow-runs/$runId`.
   - Confirm the header YOLO badge shows `DISABLED`.
   - Let the run advance until it reaches the approval gate.
   - Confirm the run pauses instead of auto-continuing.

3. Verify the live workflow-policy detail badge:
   - While that run still exists, change the workflow definition YOLO setting to `true`.
   - Refresh the same run detail page.
   - Confirm the header YOLO badge now shows `ENABLED` even though the run was originally started under `false`.

4. Verify continue/resume uses current workflow YOLO:
   - Resume or continue the existing paused run after changing the workflow setting to `true`.
   - Confirm the run no longer waits on the same approval gate and continues automatically.

5. Verify the enabled-path run behavior from a fresh run:
   - Start another new run with the workflow still set to `true`.
   - Open its run detail page.
   - Confirm the header YOLO badge shows `ENABLED`.
   - Confirm approval-gated MCP execution runs through without a manual approval pause.

6. Verify a one-step workflow is still a workflow-definition run:
   - Create or select a saved workflow definition that contains exactly one enabled child step.
   - Set `workflows.yolo_mode = false` for that workflow.
   - Start it through the workflow-definition launch path, not the direct single-step launch path.
   - Confirm the run detail YOLO indicator shows `DISABLED`.
   - Change only `step_definitions.yolo_mode` for that child step to `true`.
   - Continue, resume, or relaunch the workflow-definition run.
   - Confirm runtime behavior still follows `workflows.yolo_mode = false`.

7. Verify direct single-step behavior is delegated to Task-031:
   - Launch the same step through the direct single-step launch path.
   - Confirm effective YOLO follows current `step_definitions.yolo_mode`, not the generated runtime workflow row or `workflow_runs.yolo_mode`.

8. Verify historical run snapshots are ignored:
   - Start a workflow-definition run while `workflows.yolo_mode = false`.
   - Change `workflows.yolo_mode` to `true` after the run exists.
   - Continue, resume, or send a follow-up prompt.
   - Confirm the continuation uses the new live `workflows.yolo_mode = true`, not the historical `workflow_runs.yolo_mode`.

9. Verify operator-facing UI stays read-only:
   - On `/workflow-runs/$runId`, inspect the header, sidebar, and selected step workspace.
   - Confirm no `Step Override`, `Inherited`, or step-specific effective YOLO wording is shown.
   - Confirm any visible YOLO indicator matches the effective live SSOT for the launch mode.
   - Confirm there is no YOLO toggle, button, or other mutation control on the run detail page.
