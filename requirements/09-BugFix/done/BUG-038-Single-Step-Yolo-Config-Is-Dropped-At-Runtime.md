# BUG-038: Single-Step YOLO Config Is Dropped At Runtime

## Metadata

- Document ID: `BUG-038`
- Title: `Single-Step YOLO Config Is Dropped At Runtime`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`
- Child Documents: `none`
- Related Documents: `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`, `requirements/08-Task/todo/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md`, `requirements/09-BugFix/done/BUG-036-Workflow-Run-Yolo-State-Drifts-From-Definition.md`, `change-audit/CA-043-cp29-google-drive-mcp-manual-approval-ui.md`, `change-audit/CA-044-fix-workflow-run-yolo-default-drift.md`, `change-audit/CA-046-fix-single-step-yolo-config-drift.md`
- Replaces: `none`
- Tags: `workflow-engine, yolo, single-step, cp-29, approval, regression`

## AI Quick View

### Summary

- This bug fix corrected single-step YOLO propagation under the older Task-028 step-level YOLO model.
- Single-step launches materialize a temporary workflow and workflow step before execution starts.
- Both single-step creation paths were dropping the saved `step_definitions.yolo_mode` value when creating that temporary workflow state.
- Task-030 now supersedes this older rule for new work and simplifies current runtime behavior back to workflow-level YOLO only.

### Current Ask

- Record the fix so single-step runtime launches preserve the saved step definition YOLO value in both the admin-web runtime path and the Supabase shared runtime path.

### Key Decisions

- `V-1` Temporary single-step workflows must copy the reusable step definition YOLO setting into the generated workflow and workflow step rows.
- `V-2` Single-step runtime must not silently fall back to an unrelated default when a reusable step explicitly stores `true` or `false`.

### Constraints

- Preserve this bug record as historical evidence of the old step-level YOLO contract.
- Current runtime-rule changes should follow Task-030, not the Task-028 step precedence referenced here.
- Do not claim automated verification that could not be executed in the current shell.

### Open Questions

- Should the single-step launcher eventually surface the resolved YOLO mode in the launch UI before the run starts?

### Source Refs

- `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `supabase/functions/_shared/workflow-engine-runtime.ts`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`

## 1. Issue Summary

Single-step execution creates a runtime-generated one-step workflow from a reusable step definition. That materialization path was not copying the reusable step's saved YOLO setting into the temporary workflow rows, so the launched run could execute with approval behavior that did not match the step definition the operator had configured.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- impacted system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: FlowPilot single-step execution launched from reusable step definitions, especially Google Drive MCP steps under CP-29 approval rules
- reproduction steps:
  1. Configure a reusable step definition with an explicit YOLO default.
  2. Launch that step through the single-step execution path.
  3. Inspect the runtime-generated workflow materialization path and the resulting run behavior.
  4. Compare the saved step definition YOLO value with the temporary workflow/workflow step rows created for the run.
- frequency: deterministic whenever the single-step launch path is used

## 4. Expected vs Actual

- expected: single-step launches should preserve the reusable step definition YOLO value so runtime approval behavior matches the configured step
- actual: the temporary workflow and workflow step rows were created without copying `step_definitions.yolo_mode`, so the run launched from a drifted YOLO configuration

## 5. Impact

- users affected: operators launching reusable steps directly as single-step runs
- workflows affected: single-step runtime materialization, CP-29 Google Drive approval behavior, and any launch path that relies on reusable step YOLO defaults
- severity: medium because workflow-definition runs already preserve step overrides, but single-step launches could ignore the saved step-level intent and behave differently

## 6. Root Cause

- hypothesis: the single-step runtime path was added before step-definition YOLO defaults were fully threaded through temporary workflow creation
- confirmed cause: both `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` and `supabase/functions/_shared/workflow-engine-runtime.ts` created temporary single-step workflows using only model and reasoning data from `step_definitions`; they did not select or persist `yolo_mode`
- evidence:
  - both `createSingleStepWorkflow` implementations selected step definition fields without `yolo_mode`
  - both implementations inserted temporary `workflows` and `workflow_steps` rows without `yolo_mode`
  - Task-028 defines reusable step YOLO defaults as part of the execution contract

## 7. Fix Strategy

- `F-1` Update the admin-web runtime `createSingleStepWorkflow` path to read `step_definitions.yolo_mode` and persist it into the generated `workflows` and `workflow_steps` rows.
- `F-2` Update the Supabase shared `createSingleStepWorkflow` path to mirror the same `yolo_mode` propagation so the alternate runtime path cannot drift.
- `F-3` Add a focused regression test that asserts single-step workflow creation preserves a reusable step definition with `yolo_mode: false`.

## 8. Validation

- `V-1` Code inspection confirms both single-step creation paths now select `yolo_mode` from `step_definitions` and write it into the generated temporary workflow rows.
- `V-2` A focused runtime regression test was added to assert that a single-step step definition with `yolo_mode: false` produces temporary workflow rows with `yolo_mode: false`.
- `V-3` Attempted `npx vitest run apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`, but this shell still fails before test execution because direct file runs are not resolving the repo's `@/...` aliases in this environment.

## 9. Regression Guard

- tests:
  - added a focused runtime regression test for single-step YOLO propagation in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
  - automated execution of that test remains blocked in the current shell by repo alias resolution
- alerts:
  - none added
- audit checks:
  - keep single-step temporary workflow creation aligned with reusable step definition fields whenever runtime config expands
  - keep the admin-web runtime path and the Supabase shared runtime path behaviorally identical for single-step setup

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - Task-030 is now the active YOLO rule document for current runtime and UI work
- notes left unchanged on purpose:
  - this bug record remains historically correct for the implementation contract that existed at the time

### Historical Rule Notice

- The step-definition YOLO propagation restored by this bug was correct under the older Task-028 model.
- New rule: workflow-level YOLO only, with no step-level YOLO reads driving current execution or run-detail logic.
- See `requirements/08-Task/todo/Task-030-Workflow-Runs-Detail-Page-Yolo-Indicators.md`.
