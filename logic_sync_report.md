# Logic Sync Report - Admin MVP Skeleton

Date: 2026-05-15

## Source

- `implementation_plan.md`
- `tdd_signatures.md`
- Current `apps/admin-web` implementation

## Findings

- `[MISSING -> IMPLEMENTED]` Supabase env selection used `NEXT_PUBLIC_*` aliases. It now uses `SUPABASE_API_URL`, `SUPABASE_API_KEY`, and `SUPABASE_SERVICE_ROLE_KEY`.
- `[MISSING -> IMPLEMENTED]` `createGatewayBundle()` always returned demo repositories. It now selects Supabase repositories when all required Supabase env values exist and keeps demo fallback otherwise.
- `[MISSING -> IMPLEMENTED]` Protected routes were only grouped by folder. Protected layouts and admin APIs now use the shared admin session guard, with demo mode bypass.
- `[MISSING -> IMPLEMENTED]` Context sources only listed one hardcoded project. CRUD use cases, API routes, and page forms now support list/create/update/archive.
- `[MISSING -> IMPLEMENTED]` Feature intake used a hardcoded project id. The create form now loads projects and validates the selected project through the use case.
- `[MISSING -> IMPLEMENTED]` Workflow definitions had no pages or route. Listing/detail pages and `GET /api/workflow-definitions` now exist.
- `[MISSING -> IMPLEMENTED]` Approval listing scanned runs and lacked output preview or comments. Gateway approval queries now return enriched approval detail, and decision forms include comments.
- `[MISSING -> IMPLEMENTED]` Outputs were artifact-browser focused. The output library now lists persisted `ai_outputs`, supports detail view, approval history, versions, and markdown export.
- `[MISSING -> IMPLEMENTED]` Workflow run detail lacked context, per-step outputs, logs, and controls. The detail page now renders selected context, step output previews, logs, resume, and cancel.
- `[MISSING -> IMPLEMENTED]` Logs lacked summary/filtering. The logs page and API now support status/provider filters and dashboard summaries.
- `[MISSING -> IMPLEMENTED]` Migration lacked soft-delete, policies, indexes, and seed data. The migration now includes those pieces.
- `[PARTIAL -> IMPLEMENTED]` A Vitest harness now covers Supabase env mapping, artifact path normalization, feature-scoped context validation, and approval decision history/current step behavior. Route-level and full repository integration tests remain follow-up coverage.
