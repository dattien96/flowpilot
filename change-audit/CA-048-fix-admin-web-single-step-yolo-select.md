# CA-048 Fix Admin-Web Single-Step YOLO Select

## Scope

Closed the remaining browser launch gap in the single-step YOLO flow so the admin-web runtime now actually selects the reusable step definition `yolo_mode` column before materializing the temporary workflow rows used for execution.

## Completed

- Updated `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` so `loadStepDefinitions()` includes `yolo_mode` in the `step_definitions` select list.
- Confirmed the bug against real runtime evidence from run `d24532ef-bc25-4b80-bb38-993d9a2259ef`, where the reusable `codex_test` step definition stored `yolo_mode = true` but the generated single-step workflow, workflow step, and run rows were all persisted as `false`.
- Tightened the single-step regression test in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts` so it now asserts the query itself includes `yolo_mode`, preventing another false-positive mock from hiding a missing select column.
- Added the formal bug record at `BUG-040` documenting the incomplete admin-web single-step launch fix.

## Verification

- Direct database inspection via Supabase REST for:
  - `workflow_runs.id = d24532ef-bc25-4b80-bb38-993d9a2259ef`
  - generated `workflows.id = d3d9b750-593c-414c-b3ef-7803f043767c`
  - generated `workflow_steps.id = a8b6d152-caff-4294-913e-aa90ebc74ada`
  - `step_definitions.step_type = codex_test`
- Targeted test execution:
  - `npm test -- src/features/workflow-engine/workflow-start-runtime.test.ts`

## Residual Notes

- The specific run `d24532ef-bc25-4b80-bb38-993d9a2259ef` was launched with persisted non-YOLO runtime rows, so it remains a bad historical run and should be re-launched rather than treated as corrected in place.
- GitNexus tools were not available in this thread, so the repo's normal symbol impact analysis flow had to be replaced with careful local inspection only.

## Historical Rule Status

- This audit documents a correct fix under the earlier Task-028 step-level YOLO contract.
- Current YOLO rule: workflow-level only, no step-level YOLO reads for current execution or run-detail behavior.
- See `requirements/08-Task/done/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md`.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: BUG-040
change_type: fix
summary: Fix Admin-Web Single-Step YOLO Select
# --->8---
