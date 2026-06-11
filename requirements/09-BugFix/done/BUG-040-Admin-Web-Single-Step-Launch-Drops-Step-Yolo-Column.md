# BUG-040: Admin-Web Single-Step Launch Drops Step YOLO Column

## Metadata

- Document ID: `BUG-040`
- Title: `Admin-Web Single-Step Launch Drops Step YOLO Column`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`
- Child Documents: `none`
- Related Documents: `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`, `requirements/09-BugFix/done/BUG-038-Single-Step-Yolo-Config-Is-Dropped-At-Runtime.md`, `requirements/09-BugFix/done/BUG-039-Google-Drive-Approval-Replay-Loses-Step-Scoped-Process-Key.md`, `change-audit/CA-046-fix-single-step-yolo-config-drift.md`, `change-audit/CA-047-fix-google-drive-approval-replay-process-key.md`, `change-audit/CA-048-fix-admin-web-single-step-yolo-select.md`
- Replaces: `none`
- Tags: `workflow-engine, yolo, single-step, admin-web, cp-29, regression`

## AI Quick View

### Summary

- Task-028 requires single-step launches to preserve the reusable step definition YOLO value.
- The admin-web runtime had already been updated to write `definition.yolo_mode`, but its shared `loadStepDefinitions()` query still did not select the `yolo_mode` column.
- That meant browser-launched single-step runs materialized runtime-generated workflow rows with `false` even when the reusable step definition stored `true`.
- Run `d24532ef-bc25-4b80-bb38-993d9a2259ef` is direct evidence: the step definition was `true`, but the generated workflow, workflow step, and run rows were all `false`, so the proxy correctly required approval.

### Current Ask

- Record the fix so the admin-web single-step launch path selects `step_definitions.yolo_mode` before materializing the temporary workflow and step rows.

### Key Decisions

- `V-1` Single-step browser launches must select and preserve `step_definitions.yolo_mode` before falling back to any default.
- `V-2` Regression coverage must fail if the single-step definition query drops the `yolo_mode` column again.

### Constraints

- Keep Task-028 precedence intact: effective runtime YOLO remains `workflow_steps.yolo_mode ?? workflows.yolo_mode`.
- Keep CP-29 approval behavior unchanged; the bug is in launch-time data materialization, not in proxy approval semantics.
- Do not silently treat an omitted query column as if the step definition explicitly stored `false`.

### Open Questions

- Should the runtime launch path add a startup assertion when a reusable single-step definition has a stored `yolo_mode` but the materialized runtime workflow row does not match it?

### Source Refs

- `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- `requirements/08-Task/done/Task-028-Step-Yolo-Override.md`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
- run `d24532ef-bc25-4b80-bb38-993d9a2259ef`

## 1. Issue Summary

The admin-web single-step launch path still dropped the reusable step definition YOLO value at query time. Although `createSingleStepWorkflow` tried to copy `definition.yolo_mode`, the helper that loaded step definitions for browser-launched runs did not actually select the `yolo_mode` column. As a result, the runtime-generated single-step workflow rows silently fell back to `false`, and CP-29 approval logic correctly treated those runs as non-YOLO.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- impacted system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: browser/admin-web single-step launch path using a reusable step definition with Google Drive MCP enabled
- reproduction steps:
  1. Set a reusable step definition `yolo_mode = true`.
  2. Launch that step through the admin-web single-step flow.
  3. Inspect the runtime-generated `workflows`, `workflow_steps`, and `workflow_runs` rows.
  4. Compare them against the stored `step_definitions` row.
- frequency: deterministic for browser-launched single-step runs before this fix

## 4. Expected vs Actual

- expected: the runtime-generated workflow and workflow step rows should preserve the reusable step definition `yolo_mode` value, so a step configured with `true` launches as YOLO-enabled
- actual: the admin-web launch path loaded step definitions without the `yolo_mode` column, so the generated runtime rows fell back to `false` and the run paused for approval

## 5. Impact

- users affected: operators launching reusable steps directly from the admin-web single-step flow
- workflows affected: browser-launched single-step runs, especially CP-29 Google Drive MCP steps
- severity: high because the UI can show a step definition with YOLO enabled while the actual launched run still persists `false` and blocks on approval

## 6. Root Cause

- hypothesis: the earlier single-step YOLO fix updated the insert logic but missed the admin-web helper query that populates `definition.yolo_mode`
- confirmed cause: `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` defined `StepDefinitionRow.yolo_mode`, but `loadStepDefinitions()` still selected only `step_type, name, description, prompt_base, required_mcps, mcp_access_mode, required_skills, team_role, subagent, model, reasoning_effort` without `yolo_mode`
- evidence:
  - `step_definitions.step_type = codex_test` stored `yolo_mode = true`
  - runtime-generated `workflows.id = d3d9b750-593c-414c-b3ef-7803f043767c` stored `yolo_mode = false`
  - runtime-generated `workflow_steps.id = a8b6d152-caff-4294-913e-aa90ebc74ada` stored `yolo_mode = false`
  - `workflow_runs.id = d24532ef-bc25-4b80-bb38-993d9a2259ef` stored `yolo_mode = false`
  - the run log paused on `authGetStatus` approval because the materialized runtime state was non-YOLO

## 7. Fix Strategy

- `F-1` Add `yolo_mode` to the admin-web `loadStepDefinitions()` select list in `workflow-start-runtime.ts`.
- `F-2` Strengthen the single-step regression test so it asserts the step-definition query includes `yolo_mode`, not just that mocked rows can carry it.

## 8. Validation

- `V-1` Database inspection of run `d24532ef-bc25-4b80-bb38-993d9a2259ef` confirmed the exact mismatch between `step_definitions.yolo_mode = true` and the generated runtime rows with `false`.
- `V-2` `npm test -- src/features/workflow-engine/workflow-start-runtime.test.ts` passed from `apps/admin-web` with `32/32` tests.
- `V-3` The single-step regression test now explicitly checks that the admin-web step-definition query includes `yolo_mode`.

## 9. Regression Guard

- tests:
  - tightened the focused single-step YOLO regression test in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
- alerts:
  - none added
- audit checks:
  - whenever runtime materialization depends on a nullable step-definition field, assert the query selects that column explicitly
  - keep browser/admin-web and shared Supabase single-step launch paths aligned

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - none required because this fix restores the existing Task-028 and CP-29 contract
- notes left unchanged on purpose:
  - run `d24532ef-bc25-4b80-bb38-993d9a2259ef` remains historical evidence of the broken launch path and is not retroactively rewritten by this code fix
