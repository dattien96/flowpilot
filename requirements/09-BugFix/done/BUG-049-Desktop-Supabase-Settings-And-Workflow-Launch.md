# BUG-049: Desktop Supabase Settings And Workflow Launch

## Metadata

- Document ID: `BUG-049`
- Title: `Desktop Supabase Settings And Workflow Launch`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-14`
- Last Updated: `2026-06-14`
- Parent Documents: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md`, `requirements/10-Refactor/New-System/04-05-Phase5-Orchestration-Port-Supabase.md`, `requirements/10-Refactor/New-System/04-08-Phase8-Cutover-And-Live-Acceptance.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-043-Desktop-Workflow-Catalog-Uses-Project-Scoped-Runtime-Rows.md`, `requirements/09-BugFix/done/BUG-044-Desktop-Step-Catalog-And-Launch-Mode-Are-Workflow-Scoped.md`, `apps/desktop-flowpilot/src/clientCore.ts`, `apps/desktop-flowpilot/src/state/store.ts`, `apps/local-runner/internal/runner/interactive_handlers.go`, `apps/local-runner/internal/runner/supabase_catalog_store.go`
- Replaces: `none`
- Tags: `desktop-flowpilot, supabase, settings, workflow-launch, regression`

## AI Quick View

### Summary

- Desktop settings built admin repositories with the public anon key, so Supabase-backed settings pages could fail under RLS or admin-only table access.
- Desktop workflow launch sent the selected workflow id as the turn step id.
- The runner seeded workflow runs from the request step id directly, which produced an incorrect one-step workflow when launching a full workflow.

### Current Ask

- Restore Supabase-backed desktop settings loading and make workflow launch resolve to the workflow's first enabled concrete step.

### Key Decisions

- `V-1` Desktop settings may request the saved service-role key from the local runner and use it only for local admin repository reads/writes.
- `V-2` Full workflow launch returns the resolved first workflow step id in the run-start handle.
- `V-3` Single-step launch continues to use the selected global step definition id.

### Constraints

- Keep direct Supabase settings repositories scoped to the desktop local runtime.
- Preserve existing mock/demo behavior when no Supabase config is available.
- Keep the existing desktop run-start contract backwards compatible by making `stepId` optional in the response.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/clientCore.ts`
- `packages/flowpilot-client-core/src/data/runnerRuntimeConfigRepository.ts`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/supabase_catalog_store.go`

## 1. Issue Summary

The desktop app could authenticate against a configured Supabase workspace but fail to load data in settings areas such as workflows, projects, providers, teams, artifacts, or integrations. Workflow launches also used the selected workflow id as if it were a concrete step id, so the runner created or progressed an incorrect workflow path.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/New-System/04-05-Phase5-Orchestration-Port-Supabase.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: desktop app in runner mode against a configured local runner and Supabase workspace
- reproduction steps:
  1. Configure Supabase in the desktop settings.
  2. Open Supabase-backed settings sections.
  3. Launch a workflow from chat workflow mode.
- frequency: consistent when settings tables require admin/service-role access or when launching a full workflow.

## 4. Expected vs Actual

- expected: settings pages load the configured Supabase workspace data, and workflow mode starts at the first enabled workflow step.
- actual: settings pages could fail to load data through anon-key access, and workflow mode sent the workflow id as the step id.

## 5. Impact

- users affected: desktop users using real Supabase data
- workflows affected: settings management and full-workflow desktop launches
- severity: high because users cannot manage configured data reliably and can start the wrong workflow execution path

## 6. Root Cause

- hypothesis: the desktop retained demo/anon assumptions after the runner became the local source of runtime configuration.
- confirmed cause: `createAdminUseCases()` loaded only browser-safe runtime status and passed the anon key into `SupabaseAdminRepository`; `sendPrompt()` reused the selected workflow id as the turn `stepId`; `createRun()` seeded workflow state directly from that request `stepId`.
- evidence:
  - the runner already exposes `GET /supabase-config?includeSecret=1` for local secret-bearing config reads.
  - `sendPrompt()` sets `launchTargetId` to `selectedWorkflowId` in workflow mode and used that value in both run start and turn input.
  - `createRun()` seeded `RuntimeWorkflowStep{ID: in.StepID, StepType: in.StepID}`.

## 7. Fix Strategy

- `F-1` Add a client-core method to load the local Supabase config with the service-role key.
- `F-2` Build desktop admin Supabase repositories with the service-role key when available, falling back to anon only when no service-role key exists.
- `F-3` Add workflow-step catalog lookup support to the runner catalog.
- `F-4` Make workflow-mode run start resolve the first enabled workflow step and return that concrete `stepId`.
- `F-5` Make desktop turn submission use the runner-resolved `stepId` from the run handle.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'Test(SupabaseCatalogStoreShaping|StartRunResolvesWorkflowLaunchToFirstWorkflowStep)$'` passes in `apps/local-runner`.
- `V-2` `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `V-3` Desktop settings use the local runner secret-bearing config path before constructing admin repositories.

## 9. Regression Guard

- tests:
  - `TestStartRunResolvesWorkflowLaunchToFirstWorkflowStep`
  - `TestSupabaseCatalogStoreShaping`
- alerts:
  - none added
- audit checks:
  - desktop settings must not depend on anon-only access for admin settings tables
  - workflow-mode turns must use a concrete step id, not the workflow id

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - prior BUG-043 and BUG-044 remain valid catalog and launch-mode fixes; this bug closes the remaining runtime step-resolution and admin access gap.
