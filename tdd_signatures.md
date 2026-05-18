# TDD Signatures - Next.js to Vite + TanStack Refactor

Date: 2026-05-18

Rules for this artifact:

- signatures only
- no executable test code
- no implementation snippets
- focus on migration risk, route parity, and auth/data wiring

## 1. Vite Bootstrap And Env Wiring

### File

`apps/admin-web/src/lib/env/__tests__/app-env.test.ts`

#### `describe("hasSupabaseEnv")`

- `it("returns true only when the Vite Supabase env values are present")`
- `it("returns false when any required env value is missing")`

#### `describe("supabase env getters")`

- `it("reads the Vite env contract without relying on Next.js public env names")`
- `it("throws a clear error when a required env value is missing")`

### File

`apps/admin-web/vite.config.test.ts`

#### `describe("Vite configuration")`

- `it("resolves the app alias used by the migrated codebase")`
- `it("registers the TanStack Router plugin when required by the stack")`

## 2. Auth And Route Guards

### File

`apps/admin-web/src/features/auth/__tests__/require-auth.test.ts`

#### `describe("requireAuth")`

- `it("returns the current session when Supabase auth is available")`
- `it("redirects to /login when no session exists")`

### File

`apps/admin-web/src/routes/__tests__/authenticated-route-guard.test.ts`

#### `describe("authenticated route guard")`

- `it("blocks access to protected routes for guests")`
- `it("allows access for authenticated users")`

### File

`apps/admin-web/src/features/auth/__tests__/auth-provider.test.tsx`

#### `describe("AuthProvider")`

- `it("loads the current session on mount")`
- `it("updates session state on auth changes")`

## 3. Repository Bundle Selection

### File

`apps/admin-web/src/data/repository/__tests__/factory.test.ts`

#### `describe("createGatewayBundle")`

- `it("returns the demo bundle when Supabase env is unavailable")`
- `it("returns the Supabase bundle when Supabase env is available")`
- `it("always includes the local runner gateway")`

## 4. Domain Layer Carryover

### File

`apps/admin-web/src/domain/constant/__tests__/status.test.ts`

#### `describe("WorkflowStepStatus")`

- `it("includes skipped as a valid workflow step status")`

### File

`apps/admin-web/src/domain/usecase/workflow-runs/__tests__/list-workflow-runs-usecase.test.ts`

#### `describe("ListWorkflowRunsUseCase")`

- `it("returns runs unchanged after the framework migration")`

### File

`apps/admin-web/src/domain/usecase/projects/__tests__/list-projects-usecase.test.ts`

#### `describe("ListProjectsUseCase")`

- `it("still works through the copied domain layer")`

## 5. Routing Parity

### File

`apps/admin-web/src/routes/__tests__/route-map.test.ts`

#### `describe("route map")`

- `it("maps /login to the login route")`
- `it("maps dashboard, projects, features, workflows, approvals, outputs, artifacts, ai-runs, and settings to the R1 route tree")`
- `it("preserves nested project detail routing")`

### File

`apps/admin-web/src/routes/__tests__/redirects.test.ts`

#### `describe("root redirects")`

- `it("redirects / to /dashboard or the authenticated landing route")`
- `it("redirects unknown protected paths to the correct fallback")`

## 6. Query Hooks

### File

`apps/admin-web/src/features/projects/__tests__/queries.test.ts`

#### `describe("useProjects")`

- `it("returns the project list from the selected gateway")`

### File

`apps/admin-web/src/features/artifacts/__tests__/queries.test.ts`

#### `describe("artifact memory queries")`

- `it("returns artifact memories for the selected project or artifact")`
- `it("returns workflow prompt context items for the selected workflow run")`

## 7. Shell And Page Components

### File

`apps/admin-web/src/app/__tests__/app.test.tsx`

#### `describe("App")`

- `it("wraps the router in React Query and auth providers")`
- `it("renders the migrated shell without Next.js layout assumptions")`

### File

`apps/admin-web/src/components/layout/__tests__/app-shell.test.tsx`

#### `describe("AppShell")`

- `it("renders the navigation using TanStack Router links")`
- `it("highlights the active route correctly")`

## 8. Page-Level Regression Checks

### File

`apps/admin-web/src/routes/__tests__/pages.test.tsx`

#### `describe("migrated pages")`

- `it("renders the dashboard page")`
- `it("renders the projects page")`
- `it("renders the workflow runs page")`
- `it("renders the settings page")`
- `it("renders the artifact management and artifact memory routes")`

## 9. Local Runner Regression

### File

`apps/admin-web/src/lib/env/__tests__/workspace-root-normalization.test.ts`

#### `describe("getWorkspaceRoot")`

- `it("still resolves the workspace root correctly for local runner usage")`
- `it("does not regress path normalization when invoked from the migrated app")`

## 10. Build And Tooling Smoke Checks

### File

`apps/admin-web/package.json`

#### `describe("scripts")`

- `it("exposes dev, build, lint, and test scripts for the Vite stack")`
