# CA-003 Implement CP-07 Workflow Engine

## Scope

This audit records the workflow-engine implementation pass and the related admin-web cleanup.

It covers:

- workflow-engine planning and requirement updates
- workflow-engine domain contracts and use cases
- Supabase persistence, migrations, and edge functions
- admin-web workflow, step, and run pages
- project-scoped workflow navigation
- removal of the obsolete `/settings/integrations` page

## Completed

- [x] Refreshed the planning and spec docs for the workflow-engine program. Summary: aligned the implementation plan, coding plan, and supporting notes with the new workflow-engine scope.
- [x] Added the workflow-engine domain model and gateway contract. Summary: defined the workflow, step, run, and approval types used across the app.
- [x] Added workflow-engine use cases. Summary: separated list, detail, save, run, approval, and toggle actions into focused domain services.
- [x] Added the Supabase workflow-engine adapter and row mappers. Summary: implemented persistence for workflows, step definitions, runs, and logs.
- [x] Added schema migrations for workflow-engine tables and RLS fixes. Summary: created the workflow-engine schema, allowed global workflow writes, and added step timestamp support.
- [x] Wired workflow-engine gateways into browser and server factories. Summary: selected Supabase or in-memory workflow-engine gateways based on environment.
- [x] Added the in-memory workflow-engine gateway for demo mode. Summary: kept the non-Supabase app path functional with a local implementation.
- [x] Added Supabase edge functions for workflow run actions. Summary: implemented start-run, approval submission, and YOLO toggle execution paths.
- [x] Added the workflow realtime hook. Summary: subscribed workflow run detail views to Supabase realtime updates with polling fallback.
- [x] Added the workflow definition pages. Summary: introduced the workflows list, create page, detail page, and route tests.
- [x] Added the workflow step pages. Summary: introduced the step catalog, create page, detail page, and route tests.
- [x] Added the workflow run history page. Summary: created the workspace-level run history view with workflow and project filters.
- [x] Added project workflow navigation. Summary: exposed project-scoped workflow links from the project detail surface.
- [x] Added workflow list sorting by name and updated time. Summary: made the default sort newest update first for workflows and steps.
- [x] Removed the obsolete integrations settings page. Summary: deleted `/settings/integrations`, removed its nav entry, and updated route registration tests.

## Verification

Verified during the implementation pass:

- admin-web Vitest coverage for workflow routes, mappers, and repository behavior
- route tree regeneration for the TanStack router manifest
- `npm test -- src/routes/route-map.test.ts` in `apps/admin-web`

## Residual Notes

- The workflow-engine work now spans both admin-web and Supabase, so future changes should keep the route tree, gateway contract, and migrations in sync.
- `/settings/integrations` is intentionally removed because MCP management now lives on the dedicated MCP pages.

# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CP-07
change_type: feature
summary: Implement CP-07 Workflow Engine
# --->8---
