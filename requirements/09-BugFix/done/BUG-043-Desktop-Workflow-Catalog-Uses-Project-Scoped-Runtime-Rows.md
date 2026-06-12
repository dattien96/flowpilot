# BUG-043: Desktop Workflow Catalog Uses Project-Scoped Runtime Rows

## Metadata

- Document ID: `BUG-043`
- Title: `Desktop Workflow Catalog Uses Project-Scoped Runtime Rows`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-12`
- Last Updated: `2026-06-12`
- Parent Documents: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md`, `requirements/10-Refactor/New-System/04-05-Phase5-Orchestration-Port-Supabase.md`, `requirements/10-Refactor/New-System/04-08-Phase8-Cutover-And-Live-Acceptance.md`
- Child Documents: `none`
- Related Documents: `apps/local-runner/internal/runner/supabase_catalog_store.go`, `apps/local-runner/internal/runner/interactive_handlers.go`, `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `apps/desktop-flowpilot/src/state/store.ts`, `Screenshot 2026-06-12 at 20.39.29.png`
- Replaces: `none`
- Tags: `desktop-flowpilot, local-runner, workflow-catalog, supabase, regression`

## AI Quick View

### Summary

- The desktop workflow dropdown showed many duplicate `Single Step: ...` entries.
- The runner catalog API loaded workflows through `/client/projects/{projectId}/workflows`, but Admin Web's `/workflows` screen treats workflows as a global catalog.
- The Supabase runner catalog also failed to exclude `created_by = flowpilot-runtime`, so runtime-generated single-step workflows leaked into the picker.
- The fix adds a global `/client/workflows` route, makes the desktop call it independently of selected project, and aligns the Supabase query with Admin Web by excluding runtime workflows.

### Current Ask

- Correct desktop workflow loading so the workflow dropdown mirrors Admin Web's workflow catalog instead of showing project-scoped runtime rows.

### Key Decisions

- `V-1` Workflow definitions in the desktop selector are global catalog entries, not children of the selected project.
- `V-2` Runtime-generated workflows created by `flowpilot-runtime` must not appear in the human workflow picker.
- `V-3` Selecting a project should only bind the run/project context; it should not refetch or filter the workflow catalog.

### Constraints

- Preserve the project selector because run creation still needs project context.
- Preserve `/client/projects/{projectId}/workflows` as a compatibility alias while desktop moves to `/client/workflows`.
- Keep step loading workflow-scoped via `/client/workflows/{workflowId}/steps`.

### Open Questions

- None.

### Source Refs

- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts`
- `apps/local-runner/internal/runner/supabase_catalog_store.go`
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
- `apps/desktop-flowpilot/src/state/store.ts`
- `Screenshot 2026-06-12 at 20.39.29.png`

## 1. Issue Summary

The desktop workflow selector showed a long list of duplicate single-step workflows, including entries such as `Single Step: Business Idea`. These are runtime-generated workflow rows, not the curated workflow catalog the user sees on Admin Web at `http://localhost:3002/workflows`.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop app running in runner mode against `http://127.0.0.1:4317`
- reproduction steps:
  1. Start the runner-backed desktop.
  2. Select the `Flowpilot` project.
  3. Open the workflow dropdown.
  4. Observe many duplicated `Single Step: ...` entries.
- frequency: consistent when the Supabase workflow table contains runtime-created single-step rows

## 4. Expected vs Actual

- expected: the desktop workflow dropdown matches the Admin Web `/workflows` catalog and is not filtered by the currently selected project
- actual: the desktop loaded workflows through a project-scoped runner endpoint and included runtime-created single-step rows

## 5. Impact

- users affected: desktop users selecting workflows in runner mode
- workflows affected: any run started from the desktop workflow selector
- severity: high because users can pick the wrong runtime-generated workflow and cannot reliably find the intended catalog workflow

## 6. Root Cause

- hypothesis: the runner catalog API was not aligned with Admin Web's workflow catalog semantics
- confirmed cause: `SupabaseCatalogStore.ListWorkflows` queried `workflows` with `project_id=eq.<projectId>` and did not filter out `created_by = flowpilot-runtime`; the desktop called this project-scoped API during `selectProject`, so workflow data depended on project selection and included runtime rows
- evidence:
  - Admin Web `listWorkflows()` queries `workflows` globally and applies `.neq("created_by", "flowpilot-runtime")`
  - the runner exposed only `/client/projects/{projectId}/workflows` before the fix
  - the screenshot shows runtime-style `Single Step: ...` rows in the desktop workflow dropdown

## 7. Fix Strategy

- `F-1` Add `GET /client/workflows` as the global workflow catalog endpoint.
- `F-2` Keep `GET /client/projects/{projectId}/workflows` as a compatibility alias, but have it return the same global catalog.
- `F-3` Change `SupabaseCatalogStore.ListWorkflows` to query global workflows and exclude `created_by=flowpilot-runtime`.
- `F-4` Change the desktop `RunnerClient` contract and HTTP client to call `/client/workflows` without a project id.
- `F-5` Load workflows during navigator startup and stop refetching/filtering them when a project is selected.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'Test(SupabaseCatalogStoreShaping|ServiceListsInjectedWorkflowsGlobally|CatalogAndRegistry)$'` passed in `apps/local-runner`.
- `V-2` `npm run typecheck` passed in `apps/desktop-flowpilot`.
- `V-3` `npm run build` passed in `apps/desktop-flowpilot`.
- `V-4` Broad `go test ./internal/runner` still has unrelated local-environment failures around Google Drive MCP credentials and missing `powershell`; the catalog tests for this fix pass.

## 9. Regression Guard

- tests:
  - `TestServiceListsInjectedWorkflowsGlobally`
  - `TestSupabaseCatalogStoreShaping`
  - `TestCatalogAndRegistry`
- alerts:
  - none added
- audit checks:
  - desktop workflow list must call the global workflow catalog
  - runner Supabase catalog must exclude `flowpilot-runtime` workflow rows
  - project selection must not refetch or filter workflow definitions

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - `04-02` still documents the older project-scoped endpoint; the route remains as a compatibility alias, so no immediate upstream rewrite is required for this bugfix
