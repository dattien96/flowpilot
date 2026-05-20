# Implementation Plan - CP-07 Workflow Engine UI & Execution Dashboard

## 1. Target Outcome

Implement a canonical workflow-engine experience in `apps/admin-web` that lets a project member:

1. view all workflows for a project
2. create and edit a workflow from the 17 seeded `step_definitions`
3. reorder, enable/disable, and configure steps
4. start a workflow run with provider/model and YOLO choices
5. monitor execution through realtime run-step state
6. review waiting approvals and submit approve or reject-retry decisions
7. inspect logs and prompt-cache references used by each step

This phase establishes the frontend and data contracts for the control plane. The Go-Runner remains the executor and CP-06/CP-12 remain the deeper artifact and memory phases.

## 2. Recommended Architecture

### 2.1 Architectural choice

- Use a **parallel canonical workflow-engine slice** in `apps/admin-web/src/domain`, `data`, and `routes`.
- Do not stretch the current legacy workflow entity set to represent both old and new schemas.

### 2.2 Why this is the correct shape

- `workflow_steps` now means definition-time configuration, while `workflow_run_steps` owns execution state.
- UI needs both builder and dashboard concerns at once, and they should not share the same model.
- Later phases depend on stable canonical contracts for artifacts, logs, prompt context, and runner integration.

## 3. Proposed Package Changes

### 3.1 Domain

Add a canonical workflow-engine area under `apps/admin-web/src/domain/`:

- `model/entity/workflow-engine.ts`
  - `Workflow`
  - `WorkflowStepDefinition`
  - `WorkflowStepConfig`
  - `WorkflowRun`
  - `WorkflowRunStep`
  - `WorkflowPromptCacheEntry`
  - `WorkflowRunLog`
  - `WorkflowExecutionSummary`
- `model/payload/workflow-engine-payload.ts`
  - create/update workflow payloads
  - start run payload
  - approval decision payload
- `model/response/workflow-engine-response.ts`
  - project workflow detail
  - workflow editor view model shape
  - workflow run detail
- `gateway/workflow-engine-gateway.ts`
  - canonical query and mutation contract
- `usecase/workflow-engine/**`
  - list project workflows
  - get workflow detail
  - list step definitions
  - save workflow draft
  - start workflow run
  - list workflow runs
  - get workflow run detail
  - submit run-step approval decision

### 3.2 Data

Extend `apps/admin-web/src/data/repository/supabase/`:

- add canonical table mappers for:
  - `workflows`
  - `workflow_steps`
  - `workflow_runs`
  - `workflow_run_steps`
  - `workflow_prompt_cache`
  - `workflow_run_logs`
  - `step_definitions`
- keep implementation in the same gateway bundle initially, but isolate canonical methods behind a new `WorkflowEngineGateway` interface
- mirror the same API in demo mode so local exploration still works without Supabase env

### 3.3 Presentation / Routes

Project-scoped workflow engine routes:

- keep `/projects/$projectId/workflows` as the main entry
- extend with nested detail paths:
  - `/projects/$projectId/workflows`
  - `/projects/$projectId/workflows/$workflowId`
  - `/projects/$projectId/workflows/runs/$runId`

UI components to add under `apps/admin-web/src/components/workflow-engine/`:

- `workflow-list-panel.tsx`
- `workflow-builder-shell.tsx`
- `step-catalog-drawer.tsx`
- `workflow-step-card.tsx`
- `run-launch-dialog.tsx`
- `execution-dashboard.tsx`
- `run-step-timeline.tsx`
- `run-step-log-panel.tsx`
- `approval-decision-panel.tsx`
- `prompt-cache-badge.tsx`

## 4. Route and UX Design

### 4.1 Project workflows index

Purpose:
- show all workflows for project
- expose create CTA
- show recent runs and waiting approvals summary

Sections:
- workflow list table/cards
- built-in vs custom/template badges
- start run quick action
- recent run activity strip

### 4.2 Workflow builder detail page

Purpose:
- edit one workflow definition

Sections:
- metadata header: name, description, provider/model overrides
- ordered step stack
- add-step drawer backed by `step_definitions`
- per-step controls:
  - order index controls / drag handle
  - requires approval
  - enabled toggle
  - provider/model override
  - MCP and skill requirement summary from `step_definitions`

### 4.3 Run launch dialog

Inputs:
- project id
- workflow id
- YOLO mode
- provider/model override

Behavior:
- creates `workflow_run`
- creates ordered `workflow_run_steps`
- marks disabled definition steps as `SKIPPED` or starts them in a state the runner will skip consistently, depending on final repository strategy

### 4.4 Execution dashboard

Purpose:
- monitor one run in real time

Sections:
- run header: workflow name, status, provider, model, started by, started/finished timestamps, YOLO
- progress/timeline grouped by ordered run steps
- selected step detail pane:
  - status
  - retry count
  - rejection note
  - artifact reference placeholder
  - prompt cache reference
  - logs
- approval action panel when step status is `WAITING_USER_APPROVAL`

## 5. Data and Query Design

### 5.1 Query keys

Use canonical query keys:

- `["workflow-engine", "project", projectId, "workflows"]`
- `["workflow-engine", "workflow", workflowId]`
- `["workflow-engine", "project", projectId, "step-definitions"]`
- `["workflow-engine", "project", projectId, "runs"]`
- `["workflow-engine", "run", runId]`

### 5.2 Repository reads

Minimum repository methods:

- `listProjectWorkflows(projectId)`
- `getWorkflowDetail(workflowId)`
- `listStepDefinitions()`
- `saveWorkflow(input)`
- `deleteWorkflow(workflowId)` if included in MVP
- `startWorkflowRun(input)`
- `listProjectWorkflowRuns(projectId, filters?)`
- `getWorkflowRunDetail(runId)`
- `approveRunStep(input)`
- `rejectRunStep(input)`
- `subscribeToProjectWorkflowRuns(projectId, onChange)`
- `subscribeToWorkflowRunSteps(runId, onChange)`

### 5.3 Repository writes

Workflow save strategy:
- upsert `workflows`
- replace or diff `workflow_steps` by `workflow_id`
- preserve deterministic `order_index`

Run creation strategy:
- insert `workflow_runs`
- materialize ordered `workflow_run_steps` from enabled/disabled definition steps
- populate run-level provider/model/yolo values

Approval strategy:
- `approve`
  - set current `workflow_run_steps.status = DONE`
  - clear rejection note if needed
  - mark next runnable step `PENDING`
- `reject & retry`
  - set same `workflow_run_steps.status = PENDING`
  - write `rejection_note`
  - increment `retry_count`

## 6. Realtime Strategy

### 6.1 Channels

Use Supabase Realtime subscriptions on:

- `workflow_runs`
- `workflow_run_steps`
- optionally `workflow_run_logs` for active detail screens

### 6.2 UI response

- Index pages:
  - invalidate project run/workflow summary queries on relevant inserts/updates
- Run detail page:
  - patch or invalidate the run detail query when run-step status/log records change
- Waiting-approval panels:
  - switch actions on immediately when a step becomes `WAITING_USER_APPROVAL`

### 6.3 Guardrails

- Scope subscriptions by `projectId` and `runId` to avoid site-wide churn.
- Prefer invalidation for list pages and local patching only for active run detail if needed.

## 7. State Model Notes

### 7.1 Workflow definition vs run-step state

- `workflow_steps`:
  - definition-time only
  - no runtime status
- `workflow_run_steps`:
  - execution-time state machine
  - owns status, retry count, rejection note, artifact/prompt cache references, timestamps, error message

### 7.2 Status handling

Supported run-step statuses:

- `PENDING`
- `RUNNING`
- `WAITING_USER_APPROVAL`
- `DONE`
- `FAILED`
- `SKIPPED`

Required UX implications:

- `PENDING`: queued, not active
- `RUNNING`: highlight current step
- `WAITING_USER_APPROVAL`: show blocking approval CTA
- `DONE`: completed
- `FAILED`: show error summary and stop-state
- `SKIPPED`: visibly excluded but preserved in timeline

## 8. Migration Plan

### 8.1 Short-term coexistence

- Keep old legacy workflow pages/use cases untouched initially.
- Build canonical workflow engine under project routes first.
- Reuse shared shell/navigation/components where possible.

### 8.2 Canonical-first replacement targets

Replace placeholder:
- `apps/admin-web/src/routes/_authenticated/projects/$projectId/workflows.tsx`

Eventually migrate or retire legacy consumers:
- dashboard “latest workflow activity” widgets
- legacy approvals/output pages
- old workflow-definition/run use cases

## 9. Implementation Sequence

### Phase A - Domain and repository foundation

1. Add canonical domain entities/payloads/responses/gateway.
2. Add Supabase mapper methods for canonical tables.
3. Add demo-store canonical fixtures.
4. Add use cases for list/detail/save/start/approve/reject.

### Phase B - Workflow Builder

1. Replace placeholder project route with workflow index.
2. Add workflow detail route and builder shell.
3. Implement step catalog + ordered step editor.
4. Implement save workflow flow.

### Phase C - Execution Dashboard

1. Add run launch dialog.
2. Add run detail route and execution dashboard.
3. Show run-step timeline, log panel, prompt-cache badge, artifact placeholder link.

### Phase D - Realtime and approvals

1. Add run and run-step subscriptions.
2. Implement approve action.
3. Implement reject & retry action with required note.
4. Validate YOLO-mode behavior in UI states.

### Phase E - Legacy cleanup follow-up

1. Point dashboard summaries to canonical queries.
2. Decommission legacy workflow-definition/run assumptions where safe.

## 10. Risks and Mitigations

### Risk 1 - Legacy and canonical naming collision

Mitigation:
- new domain files should be explicitly named `workflow-engine-*`
- avoid reusing old `WorkflowStep` runtime semantics

### Risk 2 - CP-06 artifact dependency gap

Mitigation:
- model artifact references as nullable links in run-step detail UI
- ship placeholders now, full artifact detail wiring in CP-06

### Risk 3 - Approval state drift

Mitigation:
- keep approval decisions as direct run-step transitions in one use case path
- derive UI from run-step state, not a second shadow state

### Risk 4 - Realtime over-invalidation

Mitigation:
- scope channels
- use stable query keys
- limit active subscriptions to mounted project/run pages

## 11. Definition of Done For Coding Phase

- Project workflow route is no longer a placeholder.
- Users can create/edit workflows from the 17 seeded step definitions.
- Users can enable/disable/reorder steps and toggle approval requirements.
- Users can start a run with YOLO/provider/model options.
- Run detail reflects canonical `workflow_run_steps` state.
- Waiting approval steps expose approve and reject-retry actions.
- Realtime updates keep run detail current.
- New code uses canonical workflow tables and does not add new dependency on legacy `ai_outputs` runtime flow.
