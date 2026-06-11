# Task-031: Single-Step Step-Definition YOLO Policy

## Metadata

- Document ID: `Task-031`
- Title: `Single-Step Step-Definition YOLO Policy`
- Phase: `task`
- Status: `done`
- Owner: `Codex`
- Reviewers: `User`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`
- Child Documents: `none`
- Related Documents: `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`, `requirements/08-Task/done/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md`, `requirements/09-BugFix/done/BUG-038-Single-Step-Yolo-Config-Is-Dropped-At-Runtime.md`, `requirements/09-BugFix/done/BUG-040-Admin-Web-Single-Step-Launch-Drops-Step-Yolo-Column.md`, `change-audit/CA-051-single-step-step-definition-yolo-policy.md`
- Replaces: `none`
- Tags: `workflow-engine, single-step, yolo-mode, step-definitions, approval`

## AI Quick View

### Summary

- YOLO is allowed only on `workflows.yolo_mode` and `step_definitions.yolo_mode`.
- Workflow-definition runs, including workflows with exactly one child step, use `workflows.yolo_mode` as the live source of truth.
- Direct single-step launches use `step_definitions.yolo_mode` as the live source of truth.
- Run detail UI displays the current effective YOLO policy only; it must not mutate YOLO.

### Current Ask

- Finalize the YOLO SSOT rule after Task-030/Task-031 churn so future code and docs distinguish workflow-definition runs from direct single-step runs.

### Key Decisions

- `T-1` A workflow run always means `startMode = workflow-definition`, even if the workflow contains exactly one step.
- `T-2` A direct single-step run means `startMode = single-step`; it is not equivalent to a workflow containing one step.
- `T-3` Workflow-definition runs use `workflows.yolo_mode`; do not read `step_definitions.yolo_mode` for their child steps.
- `T-4` Direct single-step runs use `step_definitions.yolo_mode`; any generated runtime workflow row is an engine implementation detail, not the product SSOT.
- `T-5` Run detail and resume/follow-up behavior must always read the current live SSOT value, not historical `workflow_runs.yolo_mode`.

### Constraints

- Preserve CP-29 approval semantics: YOLO off pauses for approval, YOLO on auto-approves policy-allowed MCP calls.
- Do not restore inherited or override wording in workflow-step builder surfaces.
- Do not allow YOLO mutation from the workflow run detail UI; show one current effective YOLO indicator only.
- Any interactive YOLO mutation control added during earlier churn is superseded by this final display-only run UI rule and should be removed or disabled in implementation follow-up.
- GitNexus MCP tools were not available in this thread, so required graph impact analysis could not be executed.

### Open Questions

- None.

### Source Refs

- `requirements/08-Task/done/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md`
- `requirements/09-BugFix/done/BUG-038-Single-Step-Yolo-Config-Is-Dropped-At-Runtime.md`
- `requirements/09-BugFix/done/BUG-040-Admin-Web-Single-Step-Launch-Drops-Step-Yolo-Column.md`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `supabase/functions/_shared/workflow-engine-runtime.ts`

## 1. Goal

Finalize the YOLO source-of-truth rule so workflow-definition runs and direct single-step runs each have one clear live policy source, with no run-detail mutation.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/07-Coding-Plan/done/CP-07-Workflow-Engine-UI.md`
- tech design: `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`
- system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`
- specific upstream ids: `Task-030`, `BUG-038`, `BUG-040`, `CP-29 P-11`, `CP-29 P-12`

## 3. Trigger

Task-030 simplified YOLO to workflow-level state. That remains correct for every workflow-definition run, including a workflow definition that contains exactly one child step. Direct single-step execution is a different launch mode and uses `step_definitions.yolo_mode`.

## 4. Exact Change

- `T-1` Allow YOLO persistence only in `public.workflows.yolo_mode` and `public.step_definitions.yolo_mode`.
- `T-2` Keep `public.workflow_steps.yolo_mode` removed and do not add step-level YOLO inheritance or override semantics.
- `T-3` For `startMode = workflow-definition`, resolve effective YOLO from the current `workflows.yolo_mode` row only.
- `T-4` For `startMode = single-step`, resolve effective YOLO from the current `step_definitions.yolo_mode` row only.
- `T-5` Treat runtime-generated workflow rows for single-step execution as implementation details; they must not redefine the product distinction between single-step runs and workflow-definition runs.
- `T-6` Run detail UI must show one current effective YOLO indicator and must not expose a toggle or mutation action.
- `T-7` Continue, resume, and follow-up prompt flows must re-read the current live SSOT value. Historical `workflow_runs.yolo_mode` is not authoritative.
- `T-8` Existing interactive YOLO controls from earlier iterations are not part of the finalized rule unless a later task explicitly defines a non-run-detail configuration surface.

## 5. Touched Areas

- files: `apps/admin-web/src/domain/model/entity/workflow-engine.ts`, `apps/admin-web/src/data/repository/supabase/workflow-engine-mappers.ts`, `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts`, `apps/admin-web/src/data/repository/demo/in-memory-workflow-engine-gateway.ts`, `apps/admin-web/src/routes/_authenticated/workflow-steps/create.tsx`, `apps/admin-web/src/routes/_authenticated/workflow-steps/$stepType.tsx`, `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`, `supabase/functions/_shared/workflow-engine-runtime.ts`, `supabase/functions/workflow-engine-start-run/index.ts`, `supabase/migrations/20260611120000_restore_step_definition_single_step_yolo.sql`
- modules: `admin-web`, `workflow-engine`, `supabase edge runtime`
- routes: `/workflow-runs/$runId`
- tables: `step_definitions`, `workflows`, `workflow_runs`

## 6. Acceptance Check

- A direct single-step launch uses the selected step definition's current `yolo_mode` as the effective YOLO policy.
- A workflow-definition run, including a one-step workflow, ignores step-definition YOLO and uses the current `workflows.yolo_mode`.
- Run detail displays the current effective YOLO value from the correct SSOT table and offers no YOLO mutation control.
- Continue, resume, and follow-up flows use current live SSOT values instead of historical run snapshots.
- `workflow_steps.yolo_mode` is not restored.

## 7. Out of Scope

- Reintroducing workflow-step-level YOLO overrides.
- Adding a per-launch YOLO toggle to single-step launch forms.
- Adding a run-detail YOLO toggle or any other run-page YOLO mutation control.
- Changing CP-29 MCP approval semantics.
- Removing the generated runtime workflow implementation detail used by the current engine.

## 8. Completion Notes

- result: Finalized the docs around the single-step step-definition YOLO policy, `workflows.yolo_mode`, and the display-only run-detail rule.
- result: Previous implementation restored `step_definitions.yolo_mode` and kept `workflow_steps.yolo_mode` removed.
- verification: `npm test -- src/features/workflow-engine/workflow-start-runtime.test.ts src/data/repository/supabase/workflow-engine-mappers.test.ts src/data/repository/supabase/supabase-workflow-engine-gateway.test.ts src/routes/_authenticated/workflow-steps/create.test.tsx 'src/routes/_authenticated/workflow-steps/$stepType.test.tsx'` passed with `63` tests.
- verification: `npm run build` passed for `apps/admin-web`.
- follow-ups: GitNexus impact analysis should be run in a tool-enabled thread before commit if the MCP tools become available.
- follow-ups: Remove or disable any existing interactive YOLO mutation controls that remain from the prior implementation pass.
- upstream docs updated: `Task-031` records the accepted single-step exception to Task-030 and the finalized display-only YOLO UI rule.
