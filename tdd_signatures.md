# TDD Signatures - Admin MVP Skeleton Pending Features

Date: 2026-05-15

Rules for this artifact:

- signatures only
- no executable test code
- no implementation snippets
- prefer domain/use-case and route behavior over fragile UI snapshots

## 1. Environment And Bundle Selection

### File

`apps/admin-web/src/lib/env/__tests__/app-env.test.ts`

#### `describe("hasSupabaseEnv")`

- `it("returns true only when SUPABASE_API_URL, SUPABASE_API_KEY, and SUPABASE_SERVICE_ROLE_KEY are present")`
- `it("returns false when any required Supabase variable is missing")`

#### `describe("supabase env getters")`

- `it("reads root env values from SUPABASE_API_URL and SUPABASE_API_KEY without requiring NEXT_PUBLIC aliases")`
- `it("throws a clear error when SUPABASE_SERVICE_ROLE_KEY is requested but missing")`

### File

`apps/admin-web/src/data/repository/__tests__/factory.test.ts`

#### `describe("createGatewayBundle")`

- `it("returns the demo gateway bundle when Supabase env is unavailable")`
- `it("returns the Supabase gateway bundle when all required Supabase env values are available")`
- `it("always returns a mock workflow executor regardless of persistence mode")`

## 2. Auth And Protected Route Behavior

### File

`apps/admin-web/src/data/auth/__tests__/require-admin-session.test.ts`

#### `describe("requireAdminSession")`

- `it("returns the current authenticated user session when a valid Supabase session cookie exists")`
- `it("redirects or rejects when the request is unauthenticated")`

### File

`apps/admin-web/src/app/(protected)/__tests__/layout.test.ts`

#### `describe("(protected) layout")`

- `it("redirects guests to /login")`
- `it("renders protected content for authenticated users")`

### File

`apps/admin-web/src/app/api/__tests__/protected-routes-auth.test.ts`

#### `describe("protected admin routes")`

- `it("rejects unauthenticated POST /api/features")`
- `it("rejects unauthenticated POST /api/context-sources")`
- `it("rejects unauthenticated POST /api/workflow-runs")`
- `it("rejects unauthenticated POST /api/approvals/[approvalId]/decision")`

## 3. Context Source CRUD Use Cases

### File

`apps/admin-web/src/domain/usecase/context-sources/__tests__/create-context-source-usecase.test.ts`

#### `describe("CreateContextSourceUseCase")`

- `it("creates a project-level context source when featureId is null")`
- `it("creates a feature-scoped context source when featureId is provided")`
- `it("rejects creation when the selected project does not exist or is not visible to the user")`

### File

`apps/admin-web/src/domain/usecase/context-sources/__tests__/update-context-source-usecase.test.ts`

#### `describe("UpdateContextSourceUseCase")`

- `it("updates title, type, rawContent, and summarizedContent")`
- `it("rejects updates for invisible or missing context sources")`

### File

`apps/admin-web/src/domain/usecase/context-sources/__tests__/delete-context-source-usecase.test.ts`

#### `describe("DeleteContextSourceUseCase")`

- `it("removes or archives a context source so it no longer appears in active lists")`
- `it("does not delete context sources outside the caller visibility scope")`

### File

`apps/admin-web/src/app/api/context-sources/__tests__/route.test.ts`

#### `describe("POST /api/context-sources")`

- `it("creates a new context source from validated form or json input")`
- `it("returns validation failure for missing title, type, or rawContent")`

#### `describe("PATCH /api/context-sources/[contextSourceId]")`

- `it("updates a context source and returns the persisted row")`

#### `describe("DELETE /api/context-sources/[contextSourceId]")`

- `it("deletes or archives a context source and returns a success response")`

## 4. Feature Intake Project Selection

### File

`apps/admin-web/src/domain/usecase/features/__tests__/create-feature-usecase.test.ts`

#### `describe("CreateFeatureUseCase")`

- `it("creates a feature against the selected project id instead of a hardcoded project")`
- `it("rejects creation when the selected project id is missing or invalid")`

### File

`apps/admin-web/src/app/api/features/__tests__/route.test.ts`

#### `describe("POST /api/features")`

- `it("accepts a projectId chosen from the current project list")`
- `it("returns a validation error when projectId is empty")`

## 5. Workflow Definitions

### File

`apps/admin-web/src/domain/usecase/workflow-definitions/__tests__/list-workflow-definitions-usecase.test.ts`

#### `describe("ListWorkflowDefinitionsUseCase")`

- `it("returns active workflow definitions with version and status")`
- `it("returns ordered step definitions from the persisted definition payload")`

### File

`apps/admin-web/src/app/api/workflow-definitions/__tests__/route.test.ts`

#### `describe("GET /api/workflow-definitions")`

- `it("returns workflow definitions for authenticated users")`

## 6. Workflow Run Creation And Detail

### File

`apps/admin-web/src/domain/usecase/workflow-runs/__tests__/start-workflow-run-usecase.test.ts`

#### `describe("StartWorkflowRunUseCase")`

- `it("creates a workflow run using the selected feature and workflow definition")`
- `it("persists selectedContextSourceIds on the run")`
- `it("materializes ordered workflow steps from the definition")`
- `it("starts mock execution after run creation")`

### File

`apps/admin-web/src/domain/usecase/workflow-runs/__tests__/get-workflow-run-detail-usecase.test.ts`

#### `describe("GetWorkflowRunDetailUseCase")`

- `it("returns run, steps, selected context sources, outputs, approvals, and logs")`
- `it("groups or associates outputs by step for per-step detail rendering")`
- `it("includes metadata needed for resume/cancel placeholders")`

### File

`apps/admin-web/src/app/api/workflow-runs/__tests__/route.test.ts`

#### `describe("POST /api/workflow-runs")`

- `it("creates a workflow run and returns run detail for authenticated requests")`
- `it("returns validation failure for malformed workflow start payloads")`

## 7. Mock Workflow Executor

### File

`apps/admin-web/src/data/workflow/__tests__/mock-workflow-executor.test.ts`

#### `describe("MockWorkflowExecutor")`

- `it("completes non-approval steps and persists ai_outputs and ai_call_logs")`
- `it("pauses at approval steps and creates a pending approval")`
- `it("stores output content through the active repository implementation in Supabase mode")`
- `it("marks the run completed after the final step")`
- `it("stores an error summary when persistence fails during execution")`

## 8. Approval Center And Decision History

### File

`apps/admin-web/src/domain/usecase/approvals/__tests__/list-pending-approvals-usecase.test.ts`

#### `describe("ListPendingApprovalsUseCase")`

- `it("returns pending approvals with linked run, feature, step, and output preview data")`
- `it("sorts pending approvals by urgency or createdAt as defined by product rules")`

### File

`apps/admin-web/src/domain/usecase/approvals/__tests__/submit-approval-decision-usecase.test.ts`

#### `describe("SubmitApprovalDecisionUseCase")`

- `it("approves a pending approval, completes the approval step, and resumes workflow execution")`
- `it("stores a changes_requested decision with reviewer comment and keeps the run blocked")`
- `it("stores a rejected decision and marks the workflow run terminal")`
- `it("records decision history for every approval transition")`
- `it("rejects duplicate decisions on already decided approvals")`

### File

`apps/admin-web/src/app/api/approvals/[approvalId]/decision/__tests__/route.test.ts`

#### `describe("POST /api/approvals/[approvalId]/decision")`

- `it("accepts approved decisions and returns updated run detail")`
- `it("accepts changes_requested decisions with a comment")`
- `it("returns not found for unknown approval ids")`
- `it("returns validation failure for unsupported decision values")`

## 9. Output Library

### File

`apps/admin-web/src/domain/usecase/outputs/__tests__/list-outputs-usecase.test.ts`

#### `describe("ListOutputsUseCase")`

- `it("returns persisted ai_outputs instead of local artifact records")`
- `it("filters outputs by project, feature, run, outputType, and approval state")`

### File

`apps/admin-web/src/domain/usecase/outputs/__tests__/get-output-detail-usecase.test.ts`

#### `describe("GetOutputDetailUseCase")`

- `it("returns output content, approval state, and linked workflow metadata")`
- `it("returns version history for outputs with multiple revisions")`

### File

`apps/admin-web/src/app/api/outputs/__tests__/route.test.ts`

#### `describe("GET /api/outputs")`

- `it("returns filtered output library results for authenticated users")`

#### `describe("GET /api/outputs/[outputId]")`

- `it("returns output detail including versions and approval metadata")`

## 10. Logs And Cost Dashboard

### File

`apps/admin-web/src/domain/usecase/logs/__tests__/list-ai-call-logs-usecase.test.ts`

#### `describe("ListAiCallLogsUseCase")`

- `it("returns logs filtered by run, provider, model, status, and date range")`
- `it("does not return logs outside the caller visibility scope")`

### File

`apps/admin-web/src/domain/usecase/logs/__tests__/get-log-summary-usecase.test.ts`

#### `describe("GetLogSummaryUseCase")`

- `it("returns total token, cost, and latency summaries across the filtered log set")`
- `it("returns grouped summary data by provider and model")`

### File

`apps/admin-web/src/app/api/logs/__tests__/route.test.ts`

#### `describe("GET /api/logs")`

- `it("returns filtered logs and summary payload for authenticated users")`

## 11. Supabase Repository Adapters

### File

`apps/admin-web/src/data/repository/supabase/__tests__/context-source-gateway.test.ts`

#### `describe("Supabase context source gateway")`

- `it("maps context_sources rows to domain entities correctly")`
- `it("persists create/update/delete changes with the authenticated owner context")`

### File

`apps/admin-web/src/data/repository/supabase/__tests__/workflow-gateway.test.ts`

#### `describe("Supabase workflow gateway")`

- `it("returns workflow run detail with steps, outputs, approvals, and logs")`
- `it("creates approvals without scanning unrelated runs")`
- `it("reads workflow definitions from seeded rows")`

## 12. RLS And Seed Validation

### File

`supabase/__tests__/admin-mvp-rls.test.md`

#### `describe("admin mvp rls policies")`

- `it("allows authenticated owners to read and write their own projects and related rows")`
- `it("denies anonymous reads to protected tables")`
- `it("prevents cross-user reads of projects, features, runs, approvals, and logs")`

### File

`supabase/__tests__/admin-mvp-seed.test.md`

#### `describe("admin mvp seed data")`

- `it("seeds the feature_to_android_tech_spec workflow definition with ordered steps")`
- `it("seeds at least one project and feature for local setup validation")`

## 13. Local Runner Path Normalization Regression

### File

`apps/admin-web/src/lib/env/__tests__/workspace-root-normalization.test.ts`

#### `describe("getWorkspaceRoot path normalization")`

- `it("returns FLOWPILOT_WORKSPACE when explicitly set")`
- `it("normalizes workspace paths consistently across trailing slashes and nested cwd values")`
- `it("walks upward to the nearest directory containing .agents when FLOWPILOT_WORKSPACE is absent")`
- `it("does not regress local-runner artifact path generation on macOS-style absolute paths")`

## 14. Documentation Regression Checks

### File

`README.md`

#### `describe("documentation completeness checklist")`

- `it("documents required Supabase env variables and demo fallback behavior")`
- `it("documents auth expectations for protected admin pages")`
- `it("documents migration, seed, and test commands for apps/admin-web")`
