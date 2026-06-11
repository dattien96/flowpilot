# CA-044 Fix Workflow Run YOLO Default Drift

## Scope

Aligned workflow run startup and runtime labeling with the saved workflow YOLO definition so CP-29 runs no longer look like they have a separate runtime-only YOLO switch.

## Completed

- Fixed the Supabase `workflow-engine-start-run` function so new `workflow_runs` rows inherit `workflows.yolo_mode` instead of always starting with `yolo_mode = false`.
- Kept the existing Task-028 execution rule intact where step runtime still resolves effective YOLO as `workflow_steps.yolo_mode ?? workflows.yolo_mode`.
- Updated the workflow run detail badge text from a generic `YOLO` label to `Workflow YOLO` so the runtime page reads as workflow-level state rather than an independent live override.
- Added a formal bug record at `BUG-036` documenting the drift, the root cause split between start-run paths, and the chosen non-schema-changing fix.

## Verification

- Code inspection of both workflow run creation paths:
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
  - `supabase/functions/workflow-engine-start-run/index.ts`
- Manual compliance review of `requirements/09-BugFix/done/BUG-036-Workflow-Run-Yolo-State-Drifts-From-Definition.md`
- Attempted targeted Vitest execution:
  - `npx vitest run apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.test.ts`

## Residual Notes

- The targeted Vitest invocation in this shell currently fails before test execution because direct file runs are not resolving the repo's `@/...` path aliases in this environment.
- The run detail page still shows workflow-level YOLO only; it does not yet render the selected step's effective YOLO when a step override differs from the workflow default.
