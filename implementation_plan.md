# Implementation Plan - Admin MVP Skeleton

Date: 2026-05-15

## 1. Objective

Deliver a Next.js admin shell that persists projects, features, context sources, workflow runs, steps, outputs, approvals, and call logs in Supabase, while simulating workflow execution through a mock executor hidden behind a stable service boundary.

## 2. Architecture Decision

Use a clean-architecture shape inside `apps/admin-web`:

- Presentation: App Router pages, layouts, route handlers, server actions, client components, forms, tables, viewers
- Domain: entities, payloads, response models, use cases, gateway interfaces, workflow policies, validation rules
- Data: Supabase clients, repository implementations, auth/session adapters, realtime adapters, mock executor persistence adapters

Primary dependency direction:

- Presentation calls domain use cases
- Domain use cases depend only on gateway interfaces
- Data implements those gateway interfaces and talks to Supabase

Important rule:

- Presentation must not import Supabase repositories directly
- Domain must not import Next.js or Supabase types
- Data must not contain product workflow decisions that belong in use cases

This preserves the future ability to replace current Supabase-backed mock execution with a dedicated backend/orchestrator while keeping presentation contracts stable.

## 3. Proposed Package Structure

```text
apps/admin-web/
  src/
    app/
      (auth)/
        login/page.tsx
      (protected)/
        layout.tsx
        dashboard/page.tsx
        projects/
          page.tsx
          [projectId]/page.tsx
        features/
          page.tsx
          [featureId]/page.tsx
        context-sources/page.tsx
        workflow-runs/
          page.tsx
          [runId]/page.tsx
        approvals/page.tsx
        outputs/page.tsx
        logs/page.tsx
        settings/page.tsx
      api/
        projects/route.ts
        projects/[projectId]/route.ts
        features/route.ts
        features/[featureId]/route.ts
        context-sources/route.ts
        workflow-definitions/route.ts
        workflow-runs/route.ts
        workflow-runs/[runId]/route.ts
        workflow-runs/[runId]/resume/route.ts
        approvals/route.ts
        approvals/[approvalId]/decision/route.ts
        outputs/route.ts
        logs/route.ts
    presentation/
      components/
        layout/
        dashboard/
        projects/
        features/
        context-sources/
        workflow-runs/
        approvals/
        outputs/
        logs/
        ui/
      actions/
      mappers/
      view-models/
    domain/
      constant/
      gateway/
      model/
        entity/
        payload/
        response/
        value/
      service/
      usecase/
        projects/
        features/
        context-sources/
        workflow-runs/
        approvals/
        outputs/
        logs/
      validator/
    data/
      repository/
      datasource/
        supabase/
      auth/
      realtime/
      workflow/
      mapper/
    lib/
      env/
      utils/
```

Notes:

- `src/app` remains the Next.js entry layer, but it acts as presentation only
- route handlers and server actions should instantiate or resolve domain use cases, not repositories
- `src/domain/gateway` holds interfaces such as `ProjectGateway`, `FeatureGateway`, `WorkflowRunGateway`, `ApprovalGateway`
- `src/data/repository` holds concrete implementations such as `SupabaseProjectRepository`

## 4. Stable Entity Model

The implementation should preserve the following app-level entities and their relationships:

- `project` owns many `feature`
- `feature` belongs to one `project`
- `context_source` can belong to a `project` and optionally target a `feature`
- `workflow_definition` describes the step graph and output sequence
- `workflow_run` ties a definition to a specific project and feature
- `workflow_step` belongs to a run and represents one executable or approval node
- `ai_output` belongs to a run and usually to a step
- `approval` belongs to a step and optionally points to one output revision
- `ai_call_log` belongs to a run and step even for mock execution

Recommended table names in Supabase should remain pluralized, but code-level contracts should map cleanly to the singular entity vocabulary above.

## 5. Supabase Schema Plan

### Tables

- `profiles`
- `projects`
- `features`
- `context_sources`
- `workflow_definitions`
- `workflow_runs`
- `workflow_steps`
- `ai_outputs`
- `approvals`
- `ai_call_logs`

### Required columns by entity

#### `project`

- id
- name
- description
- platform
- repository_url
- created_by
- created_at
- updated_at

#### `feature`

- id
- project_id
- title
- business_goal
- user_problem
- expected_flow
- acceptance_criteria
- priority
- status
- owner_id
- created_at
- updated_at

#### `context_source`

- id
- project_id
- feature_id nullable
- type
- title
- raw_content
- summarized_content nullable
- tags jsonb
- created_by
- created_at

#### `workflow_definition`

- id
- name
- description
- version
- status
- definition jsonb
- created_at

#### `workflow_run`

- id
- workflow_definition_id
- project_id
- feature_id
- status
- current_step_key nullable
- selected_context_source_ids jsonb
- started_by
- started_at
- completed_at nullable
- error_summary nullable

#### `workflow_step`

- id
- workflow_run_id
- step_key
- step_name
- step_type
- status
- sequence_index
- agent_key nullable
- model_key nullable
- input_snapshot jsonb
- output_id nullable
- error_message nullable
- started_at nullable
- completed_at nullable

#### `ai_output`

- id
- workflow_run_id
- workflow_step_id
- project_id
- feature_id
- output_type
- version
- title
- content_markdown
- content_json jsonb
- validation_status
- is_approved
- created_at

#### `approval`

- id
- workflow_run_id
- workflow_step_id
- ai_output_id
- status
- reviewer_id nullable
- comment nullable
- decided_at nullable
- created_at

#### `ai_call_log`

- id
- workflow_run_id
- workflow_step_id
- provider
- model
- input_tokens
- output_tokens
- cost_estimate
- latency_ms
- status
- error_message nullable
- created_at

### RLS direction

- All user-owned rows must be scoped by authenticated user identity through `created_by`, `owner_id`, or run ownership joins
- Reads and writes should initially support a single-tenant-per-user model
- Admin-wide shared review can be introduced later by role-based policies on `profiles.role`

### Seed data

- one active `workflow_definition` named `feature_to_android_tech_spec`
- one sample `project`
- one sample `feature`
- optional starter `context_source` records for manual_text notes

## 6. UI Information Architecture

### Protected shell

- Sidebar:
  - Dashboard
  - Projects
  - Features
  - Workflow Runs
  - Approval Center
  - Outputs Library
  - Context Sources
  - Logs
  - Settings
- Top bar:
  - page title
  - current user
  - sign out

### Route responsibilities

- `dashboard`: counts and recent runs
- `projects`: list/create/edit/open
- `projects/[projectId]`: project summary, features, outputs, runs
- `features`: cross-project feature list
- `features/[featureId]`: intake detail, linked context, run start action, related outputs
- `workflow-runs`: list of historical and active runs
- `workflow-runs/[runId]`: timeline, selected output, approval panel, metadata, logs
- `approvals`: pending approvals queue
- `outputs`: searchable output library
- `logs`: mock AI call log visibility

## 7. Domain Contract Strategy

The app should expose stable request and response DTOs through domain models and use case contracts.

### Core commands

- `CreateProjectPayload`
- `UpdateProjectPayload`
- `CreateFeaturePayload`
- `UpdateFeaturePayload`
- `CreateContextSourcePayload`
- `StartWorkflowRunPayload`
- `ResumeWorkflowRunPayload`
- `SubmitApprovalDecisionPayload`

### Core queries

- `ProjectListItemDto`
- `ProjectDetailDto`
- `FeatureDetailDto`
- `WorkflowDefinitionDto`
- `WorkflowRunSummaryDto`
- `WorkflowRunDetailDto`
- `ApprovalQueueItemDto`
- `OutputLibraryItemDto`
- `AiCallLogDto`

These contracts should live in domain model packages and remain stable even if the underlying persistence or execution adapters change later.

## 8. Use Case And Gateway Boundary

Define domain use cases such as:

```text
CreateProjectUseCase
GetProjectDetailUseCase
CreateFeatureUseCase
CreateContextSourceUseCase
StartWorkflowRunUseCase
GetWorkflowRunDetailUseCase
SubmitApprovalDecisionUseCase
ListPendingApprovalsUseCase
ListOutputsUseCase
ListAiCallLogsUseCase
```

Each use case depends on gateway interfaces such as:

```text
ProjectGateway
FeatureGateway
ContextSourceGateway
WorkflowDefinitionGateway
WorkflowRunGateway
WorkflowStepGateway
AiOutputGateway
ApprovalGateway
AiCallLogGateway
WorkflowExecutorGateway
```

Current implementation in data layer:

- `SupabaseProjectRepository implements ProjectGateway`
- `SupabaseFeatureRepository implements FeatureGateway`
- `SupabaseContextSourceRepository implements ContextSourceGateway`
- `SupabaseWorkflowRunRepository implements WorkflowRunGateway`
- `SupabaseApprovalRepository implements ApprovalGateway`
- `MockWorkflowExecutorRepository or Adapter implements WorkflowExecutorGateway`

Future implementation:

- gateway implementations can call a real backend/orchestrator instead of local Supabase persistence
- use case signatures remain unchanged

Result:

- pages and route handlers stay unchanged
- only the gateway binding and some data implementations change

## 9. Workflow Definition Shape

Seed `feature_to_android_tech_spec` in `workflow_definitions.definition` as JSON:

```json
{
  "name": "feature_to_android_tech_spec",
  "version": 1,
  "steps": [
    { "key": "collect_context", "type": "tool", "requiresApproval": false },
    { "key": "generate_business_summary", "type": "ai_mock", "outputType": "business_summary" },
    { "key": "approval_business_summary", "type": "approval" },
    { "key": "generate_product_spec", "type": "ai_mock", "outputType": "product_spec" },
    { "key": "approval_product_spec", "type": "approval" },
    { "key": "generate_android_tech_spec", "type": "ai_mock", "outputType": "android_tech_spec" },
    { "key": "approval_android_tech_spec", "type": "approval" },
    { "key": "generate_task_breakdown", "type": "ai_mock", "outputType": "task_breakdown" },
    { "key": "generate_test_plan", "type": "ai_mock", "outputType": "test_plan" },
    { "key": "generate_risk_report", "type": "ai_mock", "outputType": "risk_report" }
  ]
}
```

## 10. Mock Executor Rules

### Execution behavior

- `collect_context`
  - completes immediately
  - stores selected context snapshot into step input
- generation steps
  - create one `ai_output`
  - create one `ai_call_log` with `provider=mock`
  - mark step completed
- approval steps
  - create one `approval` row with `pending`
  - set run status to `waiting_approval`
  - stop progression until a decision arrives

### Approval decisions

- `approved`
  - mark approval decided
  - mark current approval step completed
  - continue progression to next executable step
- `rejected`
  - mark approval decided
  - mark run `rejected` or `failed` according to chosen enum set
  - stop progression
- `changes_requested`
  - store comment
  - keep run at waiting state
  - no new output generation in v1

### Mock output templates

- `business_summary`
- `product_spec`
- `android_tech_spec`
- `task_breakdown`
- `test_plan`
- `risk_report`

Each template should interpolate project, feature, and context data so the output looks realistic enough for UI validation.

## 11. Realtime Plan

Use Supabase Realtime selectively through data adapters:

- subscribe to `workflow_runs` changes for run status cards and detail headers
- subscribe to `workflow_steps` for live timeline progression
- subscribe to `approvals` for approval queue refresh

Fallback:

- initial MVP can ship with query invalidation and polling if realtime wiring slows down delivery

## 12. Delivery Sequence

### Sequence 1 - Foundations

- bootstrap `apps/admin-web`
- configure Tailwind and shadcn/ui
- add env handling for Supabase
- create auth/session helpers

### Sequence 2 - Database and domain contracts

- write initial SQL migrations
- add RLS
- seed workflow definition and sample records
- define domain entities, payloads, response DTOs, and gateway interfaces
- define payload schemas with Zod at the presentation boundary and map them into domain payloads

### Sequence 3 - Domain use cases and data repositories

- implement gateways
- implement Supabase repositories
- implement project, feature, and context-source use cases
- implement workflow run and approval use cases

### Sequence 4 - Protected shell and CRUD

- login
- app layout
- dashboard shell
- project CRUD
- feature intake
- context source management

### Sequence 4 - Workflow execution slice

- workflow definition listing
- run start flow wired to `StartWorkflowRunUseCase`
- run detail timeline wired to `GetWorkflowRunDetailUseCase`
- mock execution behind `WorkflowExecutorGateway`
- approval decision flow wired to `SubmitApprovalDecisionUseCase`

### Sequence 6 - Persistence visibility

- outputs library
- logs page
- refresh-safe data loading
- optional realtime enhancements

## 13. Risks And Controls

### Risk: presentation binds directly to Supabase tables

Control:

- keep all mutation logic in domain use cases
- keep all persistence in data repositories

### Risk: mock executor leaks into component logic

Control:

- components render DTOs only
- executor behavior stays behind a domain gateway and data adapter

### Risk: unstable status enums

Control:

- centralize status unions for run, step, approval, and output validation states

### Risk: output revisions become inconsistent

Control:

- version `ai_output` records per output type and run

### Risk: approval flow is visual-only

Control:

- approval decisions always write durable `approval`, `workflow_step`, and `workflow_run` changes

## 14. MVP Completion Markers

The Admin MVP Skeleton is ready when:

- a user can sign in and reach the protected shell
- a user can create a `project`
- a user can create a `feature`
- a user can attach `context_source` records
- a user can start `feature_to_android_tech_spec`
- `workflow_run` and `workflow_step` records persist
- mock `ai_output` and `ai_call_log` rows persist
- approval gates pause progression
- approval actions resume or stop the run correctly
- outputs remain visible after refresh in the library and run detail views
- presentation depends on use cases, not repositories
- use cases depend on gateway interfaces, not Supabase
