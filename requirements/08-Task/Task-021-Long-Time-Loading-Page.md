# BUG-028 - Long Time Loading Page

## Status

Planning only. No code change has been made for this bug yet.

## Affected Pages

- `/projects/:projectId/artifacts`
- `/artifacts`
- `/workflow-runs`
- `/workflow-runs/:runId`
- `/settings/ai-providers`
- `/settings/accounts`

## Problem

The first visible render on several authenticated pages takes too long or feels blank before useful UI appears.

This is an MVP personal-tool issue, not a full SaaS scalability issue. FlowPilot is expected to be used across multiple personal devices, backed by Supabase, with local-runner/device-specific capabilities. The fix should improve first render and perceived responsiveness without changing current business logic, data ownership, route behavior, or shared Supabase semantics.

## Constraints

- Do not change business logic.
- Do not change Supabase data semantics.
- Do not change artifact sync behavior in this bug fix unless it is only moved out of the critical first-render path.
- Do not redesign the app architecture for multi-user SaaS.
- Prefer UI/loading and fetch orchestration improvements over deeper data-model changes.
- Minimize regression risk. Keep each implementation step small and independently testable.
- Keep existing page content and user-visible behavior the same after data finishes loading.

## Investigation Summary

### Shared First-Render Causes

1. The generated TanStack route tree statically imports all route files.
   - File: `apps/admin-web/src/routeTree.gen.ts`
   - Effect: the initial JavaScript bundle includes large unrelated pages, including the workflow run detail page.
   - Production build observation: one JS chunk around 1.16 MB minified and around 300 KB gzip, with Vite chunk-size warning.

2. Authenticated pages wait for the auth gate before rendering.
   - File: `apps/admin-web/src/routes/_authenticated.tsx`
   - File: `apps/admin-web/src/features/auth/require-auth.ts`
   - Effect: `requireAuth()` waits for Supabase runtime config/session before authenticated route content can render.

3. The router has no global pending component.
   - File: `apps/admin-web/src/router.tsx`
   - Effect: route loader waiting can look like a blank or late first page.

### Page-Specific Causes

#### `/projects/:projectId/artifacts`

The route loader blocks rendering until all data is loaded:

- project
- local runner artifacts
- artifact runs
- workflow runs

File: `apps/admin-web/src/routes/_authenticated/projects/$projectId/artifacts.tsx`

#### `/settings/ai-providers`

The route loader blocks rendering until all data is loaded:

- local runner health
- local providers
- supported models

File: `apps/admin-web/src/routes/_authenticated/settings/ai-providers.tsx`

#### `/artifacts`

The page renders a skeleton, but the skeleton remains until multiple requests complete:

- artifact definitions
- projects
- local runner artifacts
- artifact runs
- workflow runs

File: `apps/admin-web/src/routes/_authenticated/artifacts.tsx`

#### `/workflow-runs`

The page renders a skeleton, but loading waits for:

- workflow runs
- workflows
- projects
- workflow run title derivation
- active session reconciliation

Title derivation can inspect local artifacts and remote artifact content.

File: `apps/admin-web/src/routes/_authenticated/workflow-runs.tsx`
File: `apps/admin-web/src/lib/workflow-run-title.ts`

#### `/workflow-runs/:runId`

The detail page renders a loading state, then performs several operations:

- workflow detail
- prompt text loading
- engine detail
- local artifact listing
- artifact detail mapping
- artifact runs
- logs
- session timeout/live-session reconciliation
- realtime subscription setup and polling

File: `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`

#### `/settings/accounts`

The page uses React Query and can show a skeleton quickly, but the API response may be slow because account enrichment can perform external provider metadata calls.

Gemini account enrichment may call:

- Google OAuth token refresh
- Code Assist load endpoint
- quota endpoint

File: `apps/admin-web/src/routes/_authenticated/settings/accounts.tsx`
File: `apps/admin-web/src/app/api/local-runner/provider-accounts/route.ts`
File: `apps/admin-web/src/app/api/local-runner/provider-accounts/_account-metadata.ts`

## Root Cause Classification

### Confirmed Low-Risk Causes To Fix Now

- Oversized initial JavaScript bundle due to static route imports.
- Missing global router pending UI.
- Blocking route loaders on pages that could render skeletons first.
- Some expensive secondary data is loaded before the first useful page state.

### Higher-Risk Causes To Defer

- Changing artifact storage recovery logic.
- Changing Supabase database or storage schema.
- Changing provider account ownership/sharing rules.
- Reworking local-runner/device synchronization.
- Changing workflow run title derivation semantics.

## MVP-Safe Solution Plan

### Phase 1 - Improve First Render Without Logic Changes

Goal: make the shell and page skeleton/content appear quickly while preserving current data loading results.

1. Enable route-level code splitting for TanStack Router.
   - Expected result: reduce initial JS parse/evaluation cost.
   - Logic impact: none.
   - Regression risk: low to medium, mostly routing/build-related.
   - Validation: `npm run build`, route navigation smoke test.

2. Add a global router pending component.
   - Expected result: route loader waits show a consistent loading UI instead of appearing blank.
   - Logic impact: none.
   - Regression risk: low.
   - Validation: navigate to loader-backed routes and verify pending UI appears.

3. Keep authenticated shell behavior unchanged.
   - Do not change `requireAuth()` behavior in this bug.
   - Reason: auth changes can cause redirect/session regressions.
   - MVP decision: improve perceived loading around it, not auth semantics.

### Phase 2 - Convert Blocking Loaders To Non-Blocking Page Loading

Goal: route components should mount quickly and show page-level skeletons while fetching data.

1. Convert `/settings/ai-providers` route loader into component-level loading.
   - Preserve the same three data calls: health, providers, supported models.
   - Preserve final UI state after all data finishes.
   - Show existing/new skeleton or pending panel immediately.
   - Do not change provider install/auth/import logic.

2. Convert `/projects/:projectId/artifacts` route loader into component-level loading.
   - Preserve the same four data calls: project, local artifacts, artifact runs, workflow runs.
   - Preserve filtering logic for valid workflow run IDs.
   - Show page frame/loading state immediately.
   - Do not change artifact sync/open/download behavior.

3. Keep `/artifacts`, `/workflow-runs`, `/workflow-runs/:runId`, and `/settings/accounts` data logic intact for now.
   - They already render skeleton/loading states.
   - Later phases can reduce the work behind those skeletons.

### Phase 3 - Move Expensive Secondary Work After Primary Content

Goal: keep the same data eventually, but avoid blocking the first useful loaded state on secondary enrichment.

1. `/workflow-runs`
   - Load and render basic runs/workflows/projects first.
   - Resolve workflow run titles after the list is visible.
   - Preserve existing title derivation behavior; only defer it.
   - Do not change the title algorithm in this bug.

2. `/workflow-runs/:runId`
   - Load and render core run detail first.
   - Defer local artifact enrichment and live-session reconciliation where safe.
   - Preserve final merged outputs/session state.
   - Do not change workflow status, approval, replay, cancel, resume, or reconciliation semantics.

3. `/settings/accounts`
   - Load basic accounts first.
   - Load usage/quota metadata afterward.
   - Preserve existing metadata fields.
   - Add request timeouts only if needed and only to prevent external provider calls from holding the page indefinitely.

### Phase 4 - Deferred Deep Improvements

These are valid improvements but should not be part of the MVP bug fix unless Phase 1-3 are insufficient.

1. Replace recursive Supabase Storage scanning on first page load with background/manual recovery.
2. Introduce device-scoped local-runner state in Supabase for multi-device visibility.
3. Add artifact metadata cache or sync index.
4. Split account metadata API into separate endpoints for basic accounts and usage details.
5. Add deeper performance telemetry.

## Recommended Implementation Order

1. Route-level code splitting.
2. Global router pending UI.
3. Non-blocking loading for `/settings/ai-providers`.
4. Non-blocking loading for `/projects/:projectId/artifacts`.
5. Defer `/workflow-runs` title derivation after basic list render.
6. Defer `/settings/accounts` provider usage enrichment after basic accounts render.
7. Only then evaluate whether recursive artifact storage recovery needs a deeper redesign.

## Regression Guardrails

- Do not delete or rewrite existing data-fetching functions in the first pass.
- Prefer moving calls from route loaders to component effects or React Query while preserving call inputs and output mapping.
- Keep final rendered data equivalent after all async work finishes.
- Add or update tests around loading-state behavior and final rendered content.
- Run focused tests for each touched route.
- Run full `apps/admin-web` tests before completing the bug fix.
- Use manual smoke tests for all affected routes.

## Validation Checklist

- `/projects/:projectId/artifacts` shows shell/page loading quickly and eventually renders the same artifact list.
- `/artifacts` still shows generated artifacts, storage panel, and catalog correctly.
- `/workflow-runs` shows the run list quickly; titles may appear after the list if deferred.
- `/workflow-runs/:runId` still loads steps, outputs, logs, approvals, sessions, and action buttons correctly.
- `/settings/ai-providers` shows loading UI quickly and eventually renders the same provider/model state.
- `/settings/accounts` shows account rows quickly and eventually fills metadata/usage details.
- Build output is split into smaller chunks.
- No auth redirect behavior changes.
- No Supabase schema or storage semantics changes.
- No local-runner protocol changes.

## Definition Of Done

- First visible render is improved on all affected pages.
- No business logic changes are introduced.
- Existing route behavior remains intact after data finishes loading.
- Tests and smoke checks pass.
- Any deferred data loading is visibly safe and does not hide errors silently.

