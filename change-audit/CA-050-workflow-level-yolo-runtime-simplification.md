# CA-050 Workflow-Level YOLO Runtime Simplification

## Scope

Implemented Task-030 so workflow execution and run-detail rendering now use the current workflow-level `workflows.yolo_mode` value as the only live YOLO policy.

## Completed

- Updated `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` to stop resolving runtime YOLO from `workflow_steps.yolo_mode` or `step_definitions.yolo_mode`.
- Changed single-step runtime workflow materialization in both admin-web and Supabase shared runtime helpers to default new runtime-generated workflows to workflow-level `false` instead of copying step-level YOLO state.
- Updated `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts` and `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx` so the run detail header reads the current workflow YOLO policy instead of the historical `workflow_runs.yolo_mode` snapshot.
- Simplified `supabase/functions/_shared/workflow-engine-state-machine.ts` and `supabase/functions/_shared/workflow-engine-runtime.ts` so approval progression uses current workflow YOLO only.
- Added `supabase/migrations/20260611103000_drop_step_level_yolo_columns.sql` to drop `step_definitions.yolo_mode` and `workflow_steps.yolo_mode`.
- Removed step-level YOLO fields from the admin-web workflow-engine model, Supabase/demo mappers, and the step-definition/workflow builder screens.
- Marked Task-030 complete and updated historical Task-028, BUG-036, BUG-038, BUG-040, CA-044, CA-046, and CA-048 references to point at Task-030 as the active contract.

## Verification

- Targeted Vitest execution from `apps/admin-web`:
  - `npx vitest run --config vitest.config.ts src/features/workflow-engine/workflow-start-runtime.test.ts src/data/repository/supabase/supabase-workflow-engine-gateway.test.ts`
- Additional targeted execution that included the shared state-machine test:
  - `npm exec --prefix apps/admin-web -- vitest run --root C:/working/flowpilot/apps/admin-web --config C:/working/flowpilot/apps/admin-web/vitest.config.ts C:/working/flowpilot/supabase/functions/_shared/workflow-engine-state-machine.test.ts`
- Review pass against Task-030 acceptance:
  - current workflow YOLO is now the live runtime source
  - step-level YOLO precedence was removed from execution and approval progression
  - run-detail header reflects current workflow YOLO rather than the run snapshot
- Additional builder/schema cleanup verification:
  - `workflow-steps/create`, `workflow-steps/$stepType`, `workflows/create`, and `workflows/$workflowId` no longer render step-level YOLO controls
  - shared step-definition/workflow-step save paths no longer write step-level `yolo_mode` columns
  - new column-drop migration is present for forward deployment

## Residual Notes

- The second Vitest command also triggered many unrelated existing suite failures from the wider admin-web test surface while still showing `supabase/functions/_shared/workflow-engine-state-machine.test.ts` passing. Those failures were pre-existing and outside Task-030 scope.
- Historical `workflow_runs.yolo_mode` remains in place for compatibility and audit history, but current runtime behavior no longer treats run snapshots or step-level values as active policy.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: TASK-030
change_type: feature
summary: Workflow-Level YOLO Runtime Simplification
# --->8---
