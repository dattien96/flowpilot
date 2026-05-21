# 4C Summary - CP-07 Workflow Engine UI & Execution Dashboard

## Context

- Feature area: `apps/admin-web`
- Primary concern: replace the current project workflows placeholder and legacy workflow surfaces with a project-scoped Workflow Builder and Execution Dashboard.
- Current repo baseline:
  - Routing already exists in TanStack Router under `apps/admin-web/src/routes/_authenticated/projects/$projectId/**`.
  - `apps/admin-web/src/routes/_authenticated/projects/$projectId/workflows.tsx` is only a placeholder.
  - Workflow domain/repository code still targets the legacy MVP schema:
    - `workflow_definitions`
    - runtime `workflow_steps`
    - `workflow_runs`
    - `ai_outputs`
    - `approvals`
    - `approval_decisions`
  - CP-07 explicitly replaces that runtime model with canonical:
    - `workflows`
    - definition-time `workflow_steps`
    - `workflow_runs`
    - execution-time `workflow_run_steps`
    - `workflow_prompt_cache`
    - `workflow_run_logs`
    - `step_definitions`
  - The frontend stack is React + TanStack Router + TanStack Query + Supabase.

## Command

- Produce planning, architecture, and TDD artifacts for implementing:
  - site-wide Workflow Builder UI
  - Execution Dashboard UI
  - canonical workflow state integration
  - approval gate / reject-retry / YOLO controls
  - realtime monitoring of workflow runs and run steps
- Deliverables requested by user:
  - `4c_summary.md`
  - `task.md`
  - `implementation_plan.md`
  - `tdd_signatures.md`
- All files must be written directly to the project root.

## Constraints

- Scope is planning/architecture/TDD only. No implementation code is requested in this task.
- Planning must align to actual repo structure, not the older Android-oriented skill defaults.
- CP-07 must treat canonical workflow state as the source of truth and avoid extending the legacy `ai_outputs` runtime path.
- The design must preserve:
  - project-scoped routing
  - Supabase demo/supabase gateway split
  - TanStack Query data loading
  - Realtime updates from Supabase for run-step execution state
- Known dependency boundaries:
  - Depends on CP-05 baseline app shell and project navigation
  - Must leave room for CP-06 canonical artifacts and CP-11 Go-Runner integration
  - Must anticipate CP-12 prompt-context and artifact-memory inspection
- GitNexus instructions are present in project docs, but no GitNexus tool surface is available in this session. Planning therefore relies on direct repo inspection only.

## Criteria

- Planning output is correct when it:
  - maps CP-07 requirements into concrete admin-web modules, routes, domain models, and repository changes
  - defines at least two architectural approaches and selects one with rationale
  - breaks work into implementation phases that are executable by later coding/review agents
  - defines TDD signatures before implementation, covering:
    - workflow definition CRUD/listing
    - run start and run progression views
    - approval and reject-retry behavior
    - YOLO mode behavior
    - realtime state updates
    - prompt cache/log visibility
- Expected review mode:
  - self-review against CP-07, SD-05, SD-09, SS-04, and current admin-web structure

## Assumptions Recorded

- The primary user-facing entry point for CP-07 should remain `/projects/$projectId/workflows`.
- Site-wide means the workflow engine becomes a first-class project area, while summary widgets can surface on dashboard/project overview pages later.
- CP-07 should define new canonical gateway/entity contracts rather than trying to stretch the current legacy workflow entity model.
- Approval actions may remain UI-driven over Supabase state transitions for MVP, with the runner consuming the resulting state changes.

## Approach Options

### Option A - In-place evolution of current legacy workflow domain

- Keep existing `WorkflowGateway`, `WorkflowRun`, `WorkflowStep`, and approval/output models.
- Add compatibility fields and progressively point repository methods to canonical tables.

Pros:
- Lowest short-term churn in route code.
- Smaller first coding pass.

Cons:
- Confuses definition-time vs execution-time step concepts.
- Keeps `ai_outputs` and approval-era naming in the main surface longer.
- Increases migration risk and makes CP-06/CP-11/CP-12 harder to reason about.

### Option B - Introduce canonical workflow engine slice alongside legacy surfaces

- Add a new workflow-engine domain slice with canonical models and use cases.
- Build the new `/projects/$projectId/workflows` experience on the canonical slice.
- Keep legacy routes/use cases only for untouched old surfaces until they are retired.

Pros:
- Clean separation between old MVP skeleton and CP-07 architecture.
- Matches SD-05/SD-09 terminology exactly.
- Safer foundation for CP-06, CP-11, and CP-12.

Cons:
- More upfront modeling and repository work.
- Temporary duplication while legacy pages still exist.

### Option C - UI-first implementation with adapter layer over legacy gateway

- Build new builder/dashboard UI now, backed by adapter mappers on top of the existing workflow gateway.
- Delay full domain migration to a later CP.

Pros:
- Fast visible UI progress.
- Lower immediate data-layer disruption.

Cons:
- High technical debt.
- Direct conflict with CP-07 requirement to replace the runtime model with canonical schema.

## Recommended Approach

- Choose **Option B**.
- Reason:
  - CP-07 is not a cosmetic route change. It is the foundation for later artifact, runner, approval, and prompt-memory phases.
  - Canonical separation of workflow definition, run state, and run-step state is the main architectural requirement.
  - A parallel canonical slice lets the team ship the new builder/dashboard without destabilizing every legacy workflow page at once.
