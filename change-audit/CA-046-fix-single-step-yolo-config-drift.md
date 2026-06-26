# CA-046 Fix Single-Step YOLO Config Drift

## Scope

Aligned the single-step runtime materialization path with Task-028 so reusable step definition YOLO defaults are preserved when FlowPilot creates a temporary one-step workflow for execution.

## Completed

- Updated `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` so the admin-web single-step runtime path reads `step_definitions.yolo_mode` and persists it into the generated `workflows` and `workflow_steps` rows.
- Updated `supabase/functions/_shared/workflow-engine-runtime.ts` so the Supabase shared single-step runtime path mirrors the same `yolo_mode` propagation instead of drifting from the admin-web runtime.
- Added a focused regression test in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts` that asserts a reusable step definition with `yolo_mode: false` remains `false` in the runtime-generated single-step workflow rows.
- Added the formal bug record at `BUG-038` documenting the lost single-step YOLO configuration and the mirrored fix in both runtime paths.

## Verification

- Code inspection of both single-step workflow creation paths:
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
  - `supabase/functions/_shared/workflow-engine-runtime.ts`
- Added a focused regression test for single-step YOLO propagation:
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
- Attempted targeted test execution:
  - `npx vitest run apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`

## Residual Notes

- The targeted Vitest invocation in this shell still fails before test discovery because direct file runs are not resolving the repo's `@/...` path aliases in this environment.
- GitNexus tools were not available in this thread, so the repo's normal symbol impact analysis flow had to be replaced with careful local inspection only.

## Historical Rule Status

- This audit documents a correct fix under the earlier Task-028 step-level YOLO contract.
- Current YOLO rule: workflow-level only, no step-level YOLO reads for current execution or run-detail behavior.
- See `requirements/08-Task/done/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md`.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: TASK-028
change_type: fix
summary: Fix Single-Step YOLO Config Drift
# --->8---
