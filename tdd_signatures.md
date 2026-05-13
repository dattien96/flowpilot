# TDD Signatures - Admin MVP Skeleton

Date: 2026-05-15

Rules for this artifact:

- Signatures only
- No executable test code
- No mocks, assertions, or implementation details

## 1. Presentation Contract And Validation Tests

### File

`apps/admin-web/src/presentation/actions/__tests__/workflow-contracts.test.ts`

#### `describe("StartWorkflowRunCommandSchema")`

- `it("accepts a valid feature, workflow definition, and selected context source ids")`
  - Input:
    - valid `featureId`
    - valid `workflowDefinitionId`
    - array of `contextSourceIds`
  - Expected Output:
    - command is accepted
    - normalized payload preserves stable entity references

- `it("rejects an empty contextSourceIds array only when workflow start requires explicit context selection")`
  - Input:
    - valid feature and workflow definition ids
    - empty context source array
  - Expected Output:
    - validation behavior matches the chosen product rule

- `it("rejects malformed ids and missing required identifiers")`
  - Input:
    - missing `featureId` or `workflowDefinitionId`
    - malformed ids
  - Expected Output:
    - command is rejected with field-level validation issues

#### `describe("SubmitApprovalDecisionCommandSchema")`

- `it("accepts approved, rejected, and changes_requested decisions with optional comment")`
  - Input:
    - valid `approvalId`
    - each supported decision value
    - optional comment
  - Expected Output:
    - payload is accepted

- `it("rejects unsupported decision values")`
  - Input:
    - invalid decision string
  - Expected Output:
    - payload is rejected

## 2. Data Repository Tests

### File

`apps/admin-web/src/data/repository/__tests__/supabase-project-repository.test.ts`

#### `describe("SupabaseProjectRepository")`

- `it("creates a project owned by the authenticated user")`
  - Input:
    - user identity
    - project create payload
  - Expected Output:
    - `project` row stores ownership and timestamps

- `it("lists only projects visible to the authenticated user")`
  - Input:
    - user identity
    - mixed owned and non-owned records
  - Expected Output:
    - only allowed projects are returned

- `it("updates editable project fields without changing immutable ownership fields")`
  - Input:
    - project id
    - partial update payload
  - Expected Output:
    - allowed fields change
    - ownership fields remain stable

### File

`apps/admin-web/src/data/repository/__tests__/supabase-feature-repository.test.ts`

#### `describe("SupabaseFeatureRepository")`

- `it("creates a feature linked to its parent project")`
  - Input:
    - existing `projectId`
    - feature intake payload
  - Expected Output:
    - `feature.project_id` is stored correctly

- `it("lists features by project id")`
  - Input:
    - project id
  - Expected Output:
    - only that project's features are returned

- `it("persists acceptance criteria and expected flow rich text fields")`
  - Input:
    - populated intake fields
  - Expected Output:
    - text fields are stored and returned without truncation

### File

`apps/admin-web/src/data/repository/__tests__/supabase-context-source-repository.test.ts`

#### `describe("SupabaseContextSourceRepository")`

- `it("creates a context source attached to a feature and project")`
  - Input:
    - `projectId`
    - `featureId`
    - context create payload
  - Expected Output:
    - `context_source` row is linked to both entities

- `it("supports project-level context sources without a feature id")`
  - Input:
    - `projectId`
    - null `featureId`
  - Expected Output:
    - row persists as project-scoped context

- `it("archives or deletes context sources according to the chosen lifecycle rule")`
  - Input:
    - context source id
  - Expected Output:
    - record is no longer active for selection in workflow start

## 3. Domain Workflow Definition Tests

### File

`apps/admin-web/src/domain/service/__tests__/workflow-definition-loader.test.ts`

#### `describe("workflow definition seed and loader")`

- `it("loads the active feature_to_android_tech_spec definition")`
  - Input:
    - workflow name `feature_to_android_tech_spec`
  - Expected Output:
    - one active `workflow_definition` is returned

- `it("preserves step order and approval step placement from definition json")`
  - Input:
    - stored definition json
  - Expected Output:
    - step order remains deterministic
    - approval steps appear after the expected generation steps

- `it("rejects definitions missing required step keys or types")`
  - Input:
    - invalid definition payload
  - Expected Output:
    - validation fails before execution starts

## 4. Domain Use Case Tests

### File

`apps/admin-web/src/domain/usecase/workflow-runs/__tests__/start-workflow-run-usecase.test.ts`

#### `describe("StartWorkflowRunUseCase")`

- `it("creates a workflow_run and materializes workflow_step rows from the selected definition")`
  - Input:
    - valid start command
    - active workflow definition
  - Expected Output:
    - one `workflow_run` is created
    - one `workflow_step` per definition step is created in order

- `it("stores the selected context source ids on the workflow run")`
  - Input:
    - selected context ids
  - Expected Output:
    - run snapshot preserves selected ids for later inspection

- `it("rejects workflow start when feature and workflow definition belong to incompatible visibility scope")`
  - Input:
    - inaccessible feature or definition
  - Expected Output:
    - run creation is denied

## 5. Data Workflow Executor Tests

### File

`apps/admin-web/src/data/workflow/__tests__/mock-workflow-executor.test.ts`

#### `describe("MockWorkflowExecutor")`

- `it("completes collect_context immediately and advances to the first generation step")`
  - Input:
    - newly created run
  - Expected Output:
    - `collect_context` becomes completed
    - next step becomes current

- `it("creates ai_output and ai_call_log records for generation steps")`
  - Input:
    - runnable generation step
  - Expected Output:
    - one `ai_output` record exists
    - one `ai_call_log` record exists with mock provider metadata

- `it("pauses the workflow at approval steps with pending approval status")`
  - Input:
    - run reaching an approval step
  - Expected Output:
    - run status becomes `waiting_approval`
    - one `approval` record is created

- `it("marks the run failed when step execution encounters an unrecoverable persistence error")`
  - Input:
    - persistence failure during mock execution
  - Expected Output:
    - run status becomes terminal failure state
    - error summary is stored

## 6. Domain Approval Decision Tests

### File

`apps/admin-web/src/domain/usecase/approvals/__tests__/submit-approval-decision-usecase.test.ts`

#### `describe("SubmitApprovalDecisionUseCase")`

- `it("continues workflow execution after an approved decision")`
  - Input:
    - pending approval
    - approved decision
  - Expected Output:
    - approval is decided
    - approval step is completed
    - next executable step begins or completes

- `it("stops the workflow after a rejected decision")`
  - Input:
    - pending approval
    - rejected decision
  - Expected Output:
    - run becomes terminal rejected or failed state
    - no further steps execute

- `it("keeps the workflow waiting when changes are requested")`
  - Input:
    - pending approval
    - changes requested decision with comment
  - Expected Output:
    - approval comment is stored
    - run remains in waiting state

- `it("rejects duplicate decisions for an already decided approval")`
  - Input:
    - approval with existing decided state
  - Expected Output:
    - second decision is refused

## 7. Route Handler Tests

### File

`apps/admin-web/src/app/api/workflow-runs/__tests__/route.test.ts`

#### `describe("POST /api/workflow-runs")`

- `it("returns run summary data after successful workflow start")`
  - Input:
    - authenticated request
    - valid start payload
  - Expected Output:
    - response contains created `workflow_run` identifier and summary fields

- `it("returns validation errors for malformed workflow start payloads")`
  - Input:
    - invalid payload
  - Expected Output:
    - response contains client error status and validation detail

- `it("returns unauthorized for unauthenticated requests")`
  - Input:
    - unauthenticated request
  - Expected Output:
    - protected endpoint denies access

### File

`apps/admin-web/src/app/api/approvals/[approvalId]/decision/__tests__/route.test.ts`

#### `describe("POST /api/approvals/[approvalId]/decision")`

- `it("returns updated run detail after an approval decision is submitted")`
  - Input:
    - authenticated request
    - valid decision payload
  - Expected Output:
    - response reflects updated approval and run status

- `it("returns not found when the approval id is unknown")`
  - Input:
    - unknown approval id
  - Expected Output:
    - response indicates missing approval

## 8. Domain Query Use Case Tests

### File

`apps/admin-web/src/domain/usecase/workflow-runs/__tests__/get-workflow-run-detail-usecase.test.ts`

#### `describe("GetWorkflowRunDetailUseCase")`

- `it("returns timeline, selected output, metadata, approvals, and logs in one stable DTO")`
  - Input:
    - run id with completed and pending steps
  - Expected Output:
    - response shape is sufficient for the run detail page without client-side data stitching

- `it("orders workflow steps by sequence index")`
  - Input:
    - persisted steps in database
  - Expected Output:
    - DTO step timeline is correctly ordered

- `it("returns latest output revision metadata for each output-producing step")`
  - Input:
    - multiple output versions
  - Expected Output:
    - latest visible revision is selected according to product rule

## 9. Auth And Protected Shell Tests

### File

`apps/admin-web/src/app/(protected)/__tests__/layout.test.tsx`

#### `describe("Protected layout")`

- `it("redirects unauthenticated users to the login page")`
  - Input:
    - missing session
  - Expected Output:
    - user is redirected away from protected routes

- `it("renders sidebar navigation for authenticated users")`
  - Input:
    - valid session and profile
  - Expected Output:
    - shell renders the expected navigation items

- `it("creates or hydrates the user profile on first authenticated visit according to the chosen auth bootstrap rule")`
  - Input:
    - authenticated user without profile row
  - Expected Output:
    - profile bootstrap behavior completes successfully

## 10. Domain Output Library And Logs Tests

### File

`apps/admin-web/src/domain/usecase/outputs/__tests__/list-output-library-usecase.test.ts`

#### `describe("ListOutputLibraryUseCase")`

- `it("filters outputs by project, feature, and output type")`
  - Input:
    - filter set with project, feature, and output type
  - Expected Output:
    - only matching `ai_output` items are returned

- `it("supports approved-only filtering")`
  - Input:
    - approved-only flag
  - Expected Output:
    - only approved outputs are returned

- `it("returns related workflow run and feature metadata for each library item")`
  - Input:
    - output rows with related entities
  - Expected Output:
    - DTO contains enough metadata for list and detail links

### File

`apps/admin-web/src/domain/usecase/logs/__tests__/list-ai-call-logs-usecase.test.ts`

#### `describe("ListAiCallLogsUseCase")`

## 11. Architecture Boundary Tests

### File

`apps/admin-web/src/__tests__/architecture/clean-boundary.test.ts`

#### `describe("clean architecture dependency boundaries")`

- `it("prevents presentation modules from importing data repositories directly")`
  - Input:
    - import graph for presentation modules
  - Expected Output:
    - only domain use cases and presentation helpers are referenced

- `it("prevents domain modules from importing Next.js or Supabase modules")`
  - Input:
    - import graph for domain modules
  - Expected Output:
    - domain depends only on domain packages and shared utilities

- `it("ensures data repositories implement domain gateway interfaces")`
  - Input:
    - repository classes and gateway contracts
  - Expected Output:
    - each required repository satisfies its interface contract

- `it("lists mock ai_call_log rows by workflow run")`
  - Input:
    - run id
  - Expected Output:
    - only logs for that run are returned

- `it("preserves provider, model, token, cost, latency, and status fields for future real AI parity")`
  - Input:
    - mock log rows
  - Expected Output:
    - DTO contains the full observability shape required by the UI
