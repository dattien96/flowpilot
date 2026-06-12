# BUG-044: Desktop Step Catalog And Launch Mode Are Workflow-Scoped

## Metadata

- Document ID: `BUG-044`
- Title: `Desktop Step Catalog And Launch Mode Are Workflow-Scoped`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-12`
- Last Updated: `2026-06-12`
- Parent Documents: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md`, `requirements/10-Refactor/New-System/04-05-Phase5-Orchestration-Port-Supabase.md`, `requirements/10-Refactor/New-System/04-08-Phase8-Cutover-And-Live-Acceptance.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-043-Desktop-Workflow-Catalog-Uses-Project-Scoped-Runtime-Rows.md`, `apps/local-runner/internal/runner/supabase_catalog_store.go`, `apps/local-runner/internal/runner/interactive_handlers.go`, `apps/desktop-flowpilot/src/components/Navigator.tsx`, `apps/desktop-flowpilot/src/state/store.ts`
- Replaces: `none`
- Tags: `desktop-flowpilot, local-runner, step-catalog, launch-mode, supabase, regression`

## AI Quick View

### Summary

- The desktop step dropdown loaded `workflow_steps`, so steps depended on the selected workflow.
- Admin Web's `/workflow-steps` page is a global step-definition catalog backed by `step_definitions`.
- The desktop also allowed workflow and step selectors to imply a combined launch, but the intended startup path is a single choice: launch by workflow or launch by single step.
- The fix adds a global `/client/steps` endpoint, loads step definitions independently, and adds a radio-mode selector that enables only the chosen dropdown.

### Current Ask

- Correct the desktop startup selector so steps mirror Admin Web's `/workflow-steps` catalog and workflow/step launch is mutually exclusive.

### Key Decisions

- `V-1` Step choices are global step definitions and must not depend on selected workflow or project.
- `V-2` A new desktop session starts from exactly one launch mode: `Workflow` or `Single step`.
- `V-3` Only the dropdown for the selected launch mode is enabled.

### Constraints

- Preserve the project selector as run context.
- Preserve workflow launch mode for full workflow runs.
- Keep `/client/workflows/{workflowId}/steps` as a compatibility alias while desktop moves to `/client/steps`.

### Open Questions

- None.

### Source Refs

- `apps/admin-web/src/routes/_authenticated/workflow-steps.tsx`
- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts`
- `apps/local-runner/internal/runner/supabase_catalog_store.go`
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`

## 1. Issue Summary

The desktop step selector treated steps as children of a workflow. That does not match Admin Web, where `http://localhost:3002/workflow-steps` lists reusable global step definitions. The desktop also let the startup form imply both workflow and step selection, even though a new session should launch from one selected mode.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop app running in runner mode against `http://127.0.0.1:4317`
- reproduction steps:
  1. Start the runner-backed desktop.
  2. Select a project.
  3. Observe that the step dropdown is disabled until a workflow is selected.
  4. Compare the expected step list with Admin Web at `http://localhost:3002/workflow-steps`.
- frequency: consistent before the fix

## 4. Expected vs Actual

- expected: step selection loads the same global step-definition catalog as Admin Web's `/workflow-steps`, and startup requires choosing either workflow mode or single-step mode
- actual: steps were loaded from workflow-scoped `workflow_steps`, and the UI treated workflow plus step as a combined selection path

## 5. Impact

- users affected: desktop users starting new runner-backed sessions
- workflows affected: any desktop run launched from the workflow/step selector
- severity: high because users could not reliably launch a single reusable step and the UI taught the wrong mental model

## 6. Root Cause

- hypothesis: the desktop copied the Phase 1 workflow-step selector shape after the real catalog moved to Supabase
- confirmed cause: `RunnerClient.listSteps(workflowId)` called `/client/workflows/{workflowId}/steps`, and `SupabaseCatalogStore.ListSteps` read `workflow_steps`; the UI had no explicit launch mode, so step availability depended on workflow selection
- evidence:
  - Admin Web loads step definitions from `step_definitions` via `listStepDefinitions()`
  - desktop `Navigator` disabled the step dropdown until `selectedWorkflowId` existed
  - runner `ListSteps` previously queried `workflow_steps?workflow_id=eq.<workflowId>`

## 7. Fix Strategy

- `F-1` Add `GET /client/steps` as the global step-definition catalog endpoint.
- `F-2` Change `SupabaseCatalogStore.ListSteps` to read `step_definitions` ordered by name.
- `F-3` Change the desktop `RunnerClient` contract and clients so `listSteps()` is global.
- `F-4` Load global steps during navigator startup instead of after workflow selection.
- `F-5` Add a `Workflow` / `Single step` radio group and enable only the matching dropdown.
- `F-6` Allow `workflowId` to be omitted for single-step run creation.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'Test(SupabaseCatalogStoreShaping|ServiceListsInjectedWorkflowsGlobally|ServiceListsInjectedStepsGlobally|CatalogAndRegistry)$'` passed in `apps/local-runner`.
- `V-2` `npm run typecheck` passed in `apps/desktop-flowpilot`.
- `V-3` `npm run build` passed in `apps/desktop-flowpilot`.
- `V-4` GitNexus impact analysis for `ListSteps`, `selectWorkflow`, `sendPrompt`, and `createRun` returned `LOW`.

## 9. Regression Guard

- tests:
  - `TestServiceListsInjectedStepsGlobally`
  - `TestSupabaseCatalogStoreShaping`
  - desktop TypeScript build
- alerts:
  - none added
- audit checks:
  - desktop step list must call the global step catalog
  - step catalog must read `step_definitions`, not workflow-owned `workflow_steps`
  - workflow and step launch modes must stay mutually exclusive

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - older project/workflow-scoped endpoints remain as compatibility aliases, so the implementation can preserve backwards compatibility while desktop moves to the global catalogs
