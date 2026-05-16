# Implementation Plan - Admin MVP Skeleton Pending Features

Date: 2026-05-15

## 1. Objective

Implement the missing Admin MVP Skeleton capabilities in `apps/admin-web` so the app can persist workflow state to Supabase, protect server-rendered admin routes with Supabase Auth, preserve demo fallback, and expose richer workflow, approval, output, and log views without breaking the current clean architecture.

## 2. Key Architecture Decision

Keep the existing layering and make it real:

- presentation
  - Next.js App Router pages
  - route handlers
  - server actions or form actions
  - client components only where interactivity is required
- domain
  - entities
  - payloads and response models
  - use cases
  - gateway interfaces
  - business rules for workflow progression and approval behavior
- data
  - Supabase clients
  - repository adapters
  - auth/session adapters
  - demo adapters
  - mock workflow executor persistence adapter

Dependency rule:

- presentation -> domain use cases -> gateway interfaces -> data adapters

No page or route handler should talk to Supabase directly.

## 3. Current Gaps To Close

- `createGatewayBundle()` does not switch to a real Supabase repository bundle
- env naming is not aligned to the requested root variables
- protected routes are only a folder convention today
- feature intake and context sources use hardcoded project assumptions
- workflow gateway contracts are too narrow for the requested screens
- approval decisions do not retain proper history semantics
- outputs screen is artifact-focused instead of `ai_outputs`-focused
- logs screen does not support summaries or filtering
- migration and seed data are too thin for richer views and policies
- there is no meaningful test harness in `apps/admin-web`

## 4. Next.js Implementation Shape

### 4.1 Route groups

Keep and extend the current route groups:

```text
src/app/
  (auth)/
    login/page.tsx
  (protected)/
    layout.tsx
    dashboard/page.tsx
    projects/page.tsx
    projects/[projectId]/page.tsx
    features/page.tsx
    features/[featureId]/page.tsx
    context-sources/page.tsx
    workflow-definitions/page.tsx
    workflow-definitions/[definitionId]/page.tsx
    workflow-runs/page.tsx
    workflow-runs/[runId]/page.tsx
    approvals/page.tsx
    outputs/page.tsx
    outputs/[outputId]/page.tsx
    logs/page.tsx
    settings/page.tsx
```

### 4.2 Protected layout responsibility

`src/app/(protected)/layout.tsx` should:

- resolve the current session on the server
- redirect unauthenticated users to `/login`
- pass the signed-in user summary into shell UI if needed
- avoid doing product data fetching there

### 4.3 Protected route handlers

Protect all mutating and sensitive read APIs in `src/app/api/**` with a shared server auth guard. Start with:

- `/api/projects`
- `/api/features`
- `/api/context-sources`
- `/api/workflow-runs`
- `/api/workflow-runs/[runId]/resume`
- `/api/workflow-runs/[runId]/cancel`
- `/api/approvals/[approvalId]/decision`
- `/api/outputs`
- `/api/logs`

Use server-side auth checks before use-case invocation. Do not trust client state.

## 5. Environment And Supabase Ownership

### 5.1 Required env mapping

Use the root `.env` variables exactly as requested:

- `SUPABASE_API_URL`
- `SUPABASE_API_KEY`
- `SUPABASE_SERVICE_ROLE_KEY`

Implementation recommendation:

- add a dedicated env module that exposes:
  - `hasSupabaseEnv()`
  - `getSupabaseUrl()`
  - `getSupabaseAnonKey()` mapped from `SUPABASE_API_KEY`
  - `getSupabaseServiceRoleKey()`
- do not rename the root variables
- if the browser ever needs public exposure, mirror them internally at runtime rather than changing the root contract

### 5.2 Client split

Add three Supabase entry points in `src/data/datasource/supabase/`:

- browser/SSR auth client
  - uses `SUPABASE_API_URL` + `SUPABASE_API_KEY`
  - owns cookie/session based auth
- server request client
  - used in protected pages and protected route handlers
  - reads/writes session cookies
- service-role admin client
  - server-only
  - used by repository implementations where RLS-safe elevated operations are intentionally required
  - never leaves the server layer

### 5.3 Fallback behavior

`createGatewayBundle()` becomes the single runtime selector:

- if Supabase env is missing:
  - return demo repositories + mock executor
- if Supabase env is present:
  - return Supabase repositories + mock executor

The executor remains mock in both modes. The persistence backend changes.

## 6. Repository And Gateway Expansion

### 6.1 Keep gateway interfaces grouped by entity/use case need

Expand interfaces instead of bypassing them. Minimum additions:

- `ProjectGateway`
  - list
  - get by id
  - create
- `FeatureGateway`
  - list
  - list by project
  - get by id
  - create
- `ContextSourceGateway`
  - list by project
  - list by feature
  - get by id
  - create
  - update
  - delete or archive
- `WorkflowGateway`
  - list workflow definitions
  - get workflow definition detail
  - list runs with filters
  - get run detail including selected context, outputs by step, approvals, logs
  - create/update run
  - create/update step
  - create/update output
  - create approval decision history entries or equivalent persisted timeline
  - query approvals list/detail
  - query outputs list/detail/version history
  - query logs list/summary

### 6.2 Supabase adapters

Add concrete repository adapters in `src/data/repository/supabase/`:

- `supabase-project-gateway.ts`
- `supabase-feature-gateway.ts`
- `supabase-context-source-gateway.ts`
- `supabase-workflow-gateway.ts`
- `supabase-auth-session-gateway.ts` if a small auth abstraction is useful

### 6.3 Demo adapters

Keep the current demo bundle and expand it in parallel so demo mode can still render the new pages and interactions.

## 7. Domain Use Cases To Add Or Extend

### 7.1 Auth and session

Keep auth outside domain unless a lightweight session-reader contract is needed by presentation. Prefer a server helper in data/presentation for:

- `requireAdminSession()`
- `getOptionalSession()`

### 7.2 Context sources

Add:

- `CreateContextSourceUseCase`
- `UpdateContextSourceUseCase`
- `DeleteContextSourceUseCase`
- `GetContextSourceDetailUseCase`

### 7.3 Feature intake

Extend the features page flow:

- load projects on the server for the create form
- submit selected `projectId`
- validate the selected project exists before create

### 7.4 Workflow definitions

Add:

- `ListWorkflowDefinitionsUseCase`
- `GetWorkflowDefinitionDetailUseCase`

### 7.5 Approval center

Extend:

- `ListPendingApprovalsUseCase`
- `SubmitApprovalDecisionUseCase`

Behavior changes:

- `approved`
  - complete approval step
  - resume mock executor
- `changes_requested`
  - store reviewer comment
  - keep run blocked
  - mark current step as needs revision or continue waiting based on chosen status model
- `rejected`
  - mark run terminal

Avoid scanning every run to find one approval. Add gateway methods that address approvals directly.

### 7.6 Output library

Add:

- `ListOutputsUseCase` with filters
- `GetOutputDetailUseCase`
- `ListOutputVersionsUseCase`
- `ExportOutputUseCase` placeholder or simple markdown download contract

### 7.7 Logs and dashboard

Add:

- `ListAiCallLogsUseCase` with filters
- `GetLogSummaryUseCase`

Summary targets:

- total runs
- waiting approvals
- completed outputs
- total input/output tokens
- total estimated cost
- average latency
- grouped by provider/model/status

## 8. Supabase Schema And Migration Plan

### 8.1 Keep the existing base migration, add follow-up migration(s)

Do not rewrite the first migration in place unless the repo policy requires it. Prefer additive follow-up migration(s) that:

- add missing columns for richer UX and history
- add indexes needed for run detail, approval queue, outputs, and logs
- add RLS policies
- seed or upsert workflow definition data

### 8.2 Likely schema additions

Recommended additions or clarifications:

- `context_sources`
  - `updated_at`
  - optional `is_archived`
- `workflow_steps`
  - optional `metadata jsonb`
- `ai_outputs`
  - optional `parent_output_id` for version lineage
  - optional `export_file_path`
  - optional `updated_at`
- `approvals`
  - keep current record as current-state row
  - add separate approval-history table or encode timeline in a normalized history table if decision history needs multiple entries
- `ai_call_logs`
  - optional `metadata jsonb`
  - optional `error_message`

### 8.3 RLS direction

Policy goal for MVP:

- authenticated users can access rows they own or started
- related rows join through owned parent resources
- service-role writes are limited to server-side adapters only

If the MVP is effectively single-admin for now, keep policies simple but explicit. Do not leave tables open.

### 8.4 Seed updates

Seed:

- `profiles` entry conventions
- at least one `project`
- at least one `feature`
- sample `context_source` rows
- active `workflow_definition` for `feature_to_android_tech_spec`
- optional sample run/history rows only if they help demo mode parity

## 9. Page-Level Implementation Plan

### 9.1 Features page

- replace hidden hardcoded project field with a real project selector
- load projects and features in parallel on the server
- optionally default to the first visible project

### 9.2 Context sources page

- add list + create form + edit/delete affordances
- filter by project and optional feature
- keep create/edit mutations on protected server routes

### 9.3 Workflow definitions page

- show active definitions
- surface version, status, description, and ordered steps
- no workflow editor in this scope

### 9.4 Workflow run detail page

Add sections for:

- run header and status
- selected context sources
- ordered steps with per-step output/log state
- current and historical outputs
- approval panel with comment form
- decision history
- metadata/log summary
- placeholder buttons for resume/cancel where behavior is not implemented yet

### 9.5 Approval center

- list pending approvals with run, feature, step, and age
- include compact output preview
- include reviewer comment box
- expose approve, request changes, reject actions
- link to full run detail
- show recent decision history panel or row metadata

### 9.6 Outputs page

Reframe from artifact browser to output library:

- list persisted `ai_outputs`
- filter by project, feature, workflow run, output type, approval state
- open output detail page
- show version lineage and approval state
- offer markdown export/download

Keep local artifact browsing available only if it still supports runner/settings workflows, but it should stop being the primary meaning of "Outputs".

### 9.7 Logs page

- filter by project, run, provider, model, status, date range
- summary cards at top
- detailed table below

## 10. Next.js Best-Practice Notes For Implementation

- favor server-rendered page data loading for protected admin pages
- keep client components focused on interactive controls, not initial data orchestration
- parallelize independent server fetches with `Promise.all`
- keep route handlers thin and validation-first
- avoid leaking service-role data into client props
- keep large viewers or heavy panels isolated so they can become client components only where needed

## 11. Testing And Tooling Plan

### 11.1 Add a minimal test stack

The repo currently lacks usable test scripts in `apps/admin-web`. Add:

- unit/integration test runner for TypeScript
- jsdom only where client component testing is needed
- route/use-case tests as the first priority

Recommended baseline:

- `vitest`
- `@testing-library/react` only if needed for focused component behavior

### 11.2 Coverage priority

Write tests in this order:

1. env/factory selection and auth guards
2. use cases for context sources, workflow start, approval decisions, outputs, logs
3. route handlers for protected mutations and filtered reads
4. Supabase repository adapter tests around mapping and ownership filters
5. local-runner path normalization regression test

## 12. Documentation Updates

Update root `README.md` and `apps/admin-web/README.md` to reflect:

- required env vars
- demo mode vs Supabase mode
- auth expectations
- migrations/seed flow
- available admin pages
- test commands

## 13. Implementation Sequence

1. Add env helpers and bundle selection for demo vs Supabase
2. Add Supabase auth/session guard helpers and protect `(protected)` layout plus APIs
3. Implement Supabase repository bundle for existing project/feature/workflow reads/writes
4. Add context source CRUD use cases, routes, and page UI
5. Replace hardcoded feature project selection with real project data
6. Add workflow definitions page and gateway methods
7. Extend workflow run detail contract and page sections
8. Extend approval center behavior and history persistence
9. Rework outputs into an AI output library with detail/version/export
10. Add logs summaries and filters
11. Apply migration/RLS/seed updates
12. Add tests and fix local-runner normalization regression
13. Update docs

## 14. Done Criteria For Implementation

- protected pages and protected APIs require a valid Supabase session in Supabase mode
- project, feature, context source, workflow run, approval, output, and log state persist across refreshes in Supabase mode
- demo mode still renders usable seeded data
- hardcoded project selection is removed from feature intake
- workflow definitions page exists
- approval center and workflow detail support preview/comment/history flows
- outputs page is centered on persisted AI outputs
- logs page supports summary + filtering
- migration and seed updates are applied cleanly
- tests cover the highest-risk use cases and route handlers
