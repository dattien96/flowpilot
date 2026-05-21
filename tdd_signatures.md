# TDD Signatures - CP-07 Workflow Engine UI & Execution Dashboard

## Rules

- Signatures only.
- No runnable mocks, assertions, or implementation bodies.
- Each signature defines what to test, not how to implement it.

## 1. Domain Use Cases

### File

- `apps/admin-web/src/domain/usecase/workflow-engine/list-project-workflows-usecase.test.ts`

```ts
describe("ListProjectWorkflowsUseCase", () => {
  it("returns project workflows ordered for builder index");
  // Input:
  // - projectId with multiple workflows
  // Expected Output:
  // - workflows filtered to the project
  // - stable ordering for UI display

  it("returns template and custom workflow metadata needed by the list page");
  // Input:
  // - workflows with isTemplate true/false and override fields
  // Expected Output:
  // - list items expose template flag and override summary
});
```

### File

- `apps/admin-web/src/domain/usecase/workflow-engine/get-workflow-detail-usecase.test.ts`

```ts
describe("GetWorkflowDetailUseCase", () => {
  it("hydrates workflow definition with ordered step configs and step-definition metadata");
  // Input:
  // - workflow with ordered workflow_steps
  // - step_definitions catalog entries
  // Expected Output:
  // - builder-ready detail model with merged metadata

  it("returns null when workflow is not found");
  // Input:
  // - unknown workflowId
  // Expected Output:
  // - null result
});
```

### File

- `apps/admin-web/src/domain/usecase/workflow-engine/save-workflow-usecase.test.ts`

```ts
describe("SaveWorkflowUseCase", () => {
  it("creates a workflow and persists ordered step configs");
  // Input:
  // - create payload with metadata and multiple step configs
  // Expected Output:
  // - workflow saved
  // - order indexes preserved

  it("updates an existing workflow and replaces reordered step definitions safely");
  // Input:
  // - existing workflowId
  // - reordered, enabled/disabled, override-edited steps
  // Expected Output:
  // - workflow metadata updated
  // - stored step order matches payload

  it("rejects duplicate order indexes in the same workflow payload");
  // Input:
  // - two steps with same orderIndex
  // Expected Output:
  // - validation failure
});
```

### File

- `apps/admin-web/src/domain/usecase/workflow-engine/start-workflow-run-usecase.test.ts`

```ts
describe("StartWorkflowRunUseCase", () => {
  it("creates a run with run-level provider model and yolo settings");
  // Input:
  // - workflowId, projectId, provider/model override, yolo flag
  // Expected Output:
  // - workflow_run contains persisted execution settings

  it("materializes workflow_run_steps from workflow step definitions in order");
  // Input:
  // - workflow with ordered steps
  // Expected Output:
  // - run steps are created in matching order
  // - each run step references the source workflow_step

  it("marks disabled definition steps as skipped or non-runnable according to repository policy");
  // Input:
  // - workflow containing disabled steps
  // Expected Output:
  // - disabled steps do not block run progression
});
```

### File

- `apps/admin-web/src/domain/usecase/workflow-engine/get-workflow-run-detail-usecase.test.ts`

```ts
describe("GetWorkflowRunDetailUseCase", () => {
  it("returns run header data, ordered run steps, logs, and prompt cache references");
  // Input:
  // - run with related workflow_run_steps, workflow_run_logs, workflow_prompt_cache
  // Expected Output:
  // - dashboard-ready detail model

  it("includes rejection and retry state for a previously rejected step");
  // Input:
  // - run step with rejectionNote and retryCount > 0
  // Expected Output:
  // - detail model exposes retry context
});
```

### File

- `apps/admin-web/src/domain/usecase/workflow-engine/submit-run-step-approval-decision-usecase.test.ts`

```ts
describe("SubmitRunStepApprovalDecisionUseCase", () => {
  it("approves a waiting step and advances the next runnable step");
  // Input:
  // - current step in WAITING_USER_APPROVAL
  // Expected Output:
  // - current step becomes DONE
  // - next runnable step becomes PENDING

  it("rejects a waiting step and records rejection note for retry");
  // Input:
  // - current step in WAITING_USER_APPROVAL
  // - rejection reason text
  // Expected Output:
  // - same step becomes PENDING
  // - rejection note stored
  // - retry count incremented

  it("rejects empty rejection notes for reject-and-retry");
  // Input:
  // - reject decision with blank note
  // Expected Output:
  // - validation failure
});
```

## 2. Repository Mapping

### File

- `apps/admin-web/src/data/repository/supabase/workflow-engine-mappers.test.ts`

```ts
describe("workflow engine supabase mappers", () => {
  it("maps canonical workflow rows into domain workflow entities");
  // Input:
  // - workflows row with overrides and template flag
  // Expected Output:
  // - domain Workflow shape

  it("maps definition-time workflow_steps rows distinctly from execution-time workflow_run_steps rows");
  // Input:
  // - one row from each table
  // Expected Output:
  // - step config vs run-step entities remain structurally distinct

  it("maps workflow_run_logs and prompt cache records for dashboard rendering");
  // Input:
  // - log rows and cache rows
  // Expected Output:
  // - UI-ready log/cache entities
});
```

### File

- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.test.ts`

```ts
describe("Supabase workflow engine gateway", () => {
  it("loads project workflows from canonical workflows table");
  // Input:
  // - projectId
  // Expected Output:
  // - query targets workflows and returns only project-owned rows

  it("loads workflow detail with workflow_steps and step_definitions");
  // Input:
  // - workflowId
  // Expected Output:
  // - ordered step configs plus catalog metadata

  it("creates workflow run and workflow_run_steps in one start flow");
  // Input:
  // - start run payload
  // Expected Output:
  // - workflow_runs insert
  // - workflow_run_steps inserts

  it("updates waiting approval step to pending with rejection note on retry");
  // Input:
  // - reject decision payload
  // Expected Output:
  // - status, rejectionNote, retryCount persisted correctly
});
```

## 3. Route Loaders and Screen Behavior

### File

- `apps/admin-web/src/routes/_authenticated/projects/$projectId/workflows.test.tsx`

```ts
describe("project workflows route", () => {
  it("renders workflow list, create action, and recent run summary");
  // Input:
  // - project workflow query with multiple workflows
  // Expected Output:
  // - workflow list visible
  // - create CTA visible
  // - recent execution summary visible

  it("shows empty state when project has no workflows");
  // Input:
  // - empty workflow list
  // Expected Output:
  // - onboarding empty state for workflow creation
});
```

### File

- `apps/admin-web/src/routes/_authenticated/projects/$projectId/workflows.$workflowId.test.tsx`

```ts
describe("workflow builder detail route", () => {
  it("renders ordered step cards from workflow detail");
  // Input:
  // - workflow detail with multiple steps
  // Expected Output:
  // - step cards appear in order

  it("surfaces step catalog metadata for MCP and skill requirements");
  // Input:
  // - step definitions containing requiredMcps and requiredSkills
  // Expected Output:
  // - requirement summaries shown in builder

  it("disables invalid save when workflow has no enabled steps");
  // Input:
  // - workflow draft with all steps disabled
  // Expected Output:
  // - save blocked or validation state shown
});
```

### File

- `apps/admin-web/src/routes/_authenticated/projects/$projectId/workflows.runs.$runId.test.tsx`

```ts
describe("workflow execution dashboard route", () => {
  it("renders run header and ordered run-step timeline");
  // Input:
  // - run detail with ordered run steps
  // Expected Output:
  // - timeline mirrors canonical order and statuses

  it("shows approval actions only for waiting-user-approval step");
  // Input:
  // - selected run step in WAITING_USER_APPROVAL
  // Expected Output:
  // - approve and reject-retry controls visible

  it("shows logs and prompt cache reference for selected step");
  // Input:
  // - step with logs and promptCacheId
  // Expected Output:
  // - log panel and cache badge visible

  it("shows yolo mode as read-only execution context in run header");
  // Input:
  // - run with yoloMode true
  // Expected Output:
  // - header communicates approval-skipping mode
});
```

## 4. Realtime Behavior

### File

- `apps/admin-web/src/presentation/hooks/use-workflow-run-realtime.test.ts`

```ts
describe("useWorkflowRunRealtime", () => {
  it("invalidates active run detail when workflow_run_steps status changes");
  // Input:
  // - realtime event for tracked runId
  // Expected Output:
  // - run detail query refresh triggered

  it("ignores realtime events for other runs or projects");
  // Input:
  // - unrelated run/project event
  // Expected Output:
  // - no local invalidation

  it("refreshes logs panel when a new workflow_run_log arrives for the selected run");
  // Input:
  // - realtime log insert event for selected run
  // Expected Output:
  // - logs query refresh triggered
});
```

## 5. Demo Mode Parity

### File

- `apps/admin-web/src/data/repository/demo/demo-workflow-engine-gateway.test.ts`

```ts
describe("demo workflow engine gateway", () => {
  it("provides step definitions and project workflows for local exploration");
  // Input:
  // - demo mode bootstrapped store
  // Expected Output:
  // - builder pages can render without Supabase

  it("simulates approval and reject-retry state transitions consistently with canonical statuses");
  // Input:
  // - waiting approval demo run step
  // Expected Output:
  // - approve and retry paths match canonical behavior
});
```
