# CP-15 Implementation Plan

## Scope

This plan covers the CP-15 migration work needed to make project-based workflow execution real in the current codebase:

- required project path selection flow
- project detail 3-mode workflow trigger UI
- step definition `skills` / `subagent` / `model` support
- workflow start payload and runtime contract changes
- Go runner prompt assembly and model-family routing
- MCP gating before execution
- artifact input/output handoff
- targeted tests

This plan intentionally does **not** create TDD signatures.

## Architecture Direction

### Recommended approach

Keep the existing split:

- `apps/admin-web`: authoring UI, start/run entrypoints, local-runner client
- `supabase/functions`: authoritative workflow-run creation and step-to-step progression
- `apps/local-runner`: machine-local execution, model CLI routing, artifact file IO

Do not introduce a second orchestration layer in the web app. The web should validate input and invoke one runtime start contract; the edge runtime should create run state and delegate execution to the local runner; the runner should stay responsible for provider/model command selection and workspace-bound artifact handling.

### Naming rule

The repo currently uses `directoryPath` / `directory_path` on project models and schema. CP-15 language says `project_path`. For this implementation, keep the persisted field name as `directory_path` unless a separate rename CP is approved. Treat it as the required project workspace path.

## Current Gaps

- Project creation/editing still treats `directoryPath` as optional.
- Project detail routes still link out to workflow pages instead of owning workflow launch.
- `StartWorkflowRunUseCase` and `workflow-engine-start-run` only support `workflowId + projectId`.
- `StepDefinition` only supports `requiredMcps`, `requiredSkills`, and `agentType`.
- Workflow runtime does not yet pass begin prompt, single-step execution mode, or artifact bindings into execution.
- Go runner `ExecutePrompt` only supports `providerKey === "codex"` and does not assemble prompts from step contracts.

## Target Runtime Shape

### Start contract

One start payload should support both full-workflow and single-step execution:

- `projectId`
- `workflowId`
- `startMode`: `"workflow"` | `"single_step"`
- `stepType?` or `workflowStepId?` for single-step mode
- `beginPrompt`
- `triggerSource`: `"project_detail"`

### Step execution contract

Each executable step should resolve to:

- project workspace path
- step definition metadata: `name`, `description`, `requiredMcps`, `skills`, `subagent`, `model`
- resolved input artifacts
- declared output artifact bindings
- run/step ids for artifact bookkeeping

### Model routing rule

Derive provider command from model family only:

- `gpt-*` -> Codex CLI adapter
- `gemini-*` -> Gemini CLI adapter
- `claude-*` -> Claude CLI adapter

Workflow-level `provider_override` / `model_override` should become compatibility fallbacks, not the primary source of truth.

## Workstreams

### 1. Project path requirement and picker flow

Objective: block project creation without a real local workspace path and make project edit/create use the same validation model.

Files:

- `apps/admin-web/src/domain/model/entity/project.ts`
- `apps/admin-web/src/domain/model/payload/project-payload.ts`
- `apps/admin-web/src/domain/gateway/project-gateway.ts`
- `apps/admin-web/src/domain/usecase/projects/create-project-usecase.ts`
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`
- `apps/admin-web/src/data/repository/demo/demo-gateway-bundle.ts`
- `apps/admin-web/src/routes/_authenticated/projects/create.tsx`
- `apps/admin-web/src/routes/_authenticated/projects/$projectId.tsx`
- `apps/admin-web/src/app/api/projects/route.ts`
- `supabase/migrations/*new_cp15_project_path*.sql`

Plan:

- Make `directoryPath` required in create payloads and UI submit paths.
- Update Supabase project creation to actually persist `directory_path`; it currently drops the field.
- Add a CP-15 migration in two phases:
  - backfill support for legacy null rows
  - enforce `directory_path is not null` only after data is cleaned
- Introduce a small browser-safe directory selection abstraction in admin web:
  - native/desktop bridge if available
  - manual text input fallback when bridge is unavailable
- Show explicit errors for permission denial, bridge absence, and invalid path selection.
- Keep project edit screen able to repair missing paths on legacy rows.

Design note:

- Do not bury path picking inside a generic form helper. It should be a reusable project-workspace selector component because the project detail page will depend on the same affordance.

### 2. Project detail workflow launch surface

Objective: move workflow execution entry to project detail and support the 3 required modes.

Files:

- `apps/admin-web/src/routes/_authenticated/projects/$projectId.tsx`
- `apps/admin-web/src/routes/_authenticated/projects/$projectId.test.tsx`
- `apps/admin-web/src/domain/usecase/workflow-engine/start-workflow-run-usecase.ts`
- `apps/admin-web/src/domain/gateway/workflow-engine-gateway.ts`
- `apps/admin-web/src/domain/model/payload/workflow-payload.ts`
- optional new component folder:
  - `apps/admin-web/src/presentation/components/projects/project-workflow-launcher.tsx`

Plan:

- Replace the current link-only “Trigger workflow” / “Create private workflow” actions with one launcher block on the project detail page.
- Add mode state:
  - start from workflow definition
  - start from single step
  - create new workflow
- For workflow mode:
  - load project-visible workflows
  - require begin prompt
  - submit unified start payload
- For single-step mode:
  - load shared step definitions
  - require begin prompt
  - submit unified start payload with `startMode=single_step`
- For create-new-workflow mode:
  - navigate to workflow creation with `projectId`
  - preserve return path back to project detail
- Disable start actions when project path is missing.

Design note:

- The project detail route already loads project, teams, and workflow runs. Extend that loader to also fetch workflows and step definitions instead of adding a second ad hoc fetch layer inside the UI.

### 3. Step definition runtime contract expansion

Objective: make step definitions the source of truth for execution guidance and model selection.

Files:

- `apps/admin-web/src/domain/model/entity/workflow-engine.ts`
- `apps/admin-web/src/data/repository/supabase/workflow-engine-mappers.ts`
- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts`
- `apps/admin-web/src/data/repository/demo/in-memory-workflow-engine-gateway.ts`
- `apps/admin-web/src/routes/_authenticated/workflow-steps/create.tsx`
- `apps/admin-web/src/routes/_authenticated/workflow-steps/$stepType.tsx`
- `apps/admin-web/src/routes/_authenticated/workflow-steps/*.test.tsx`
- `supabase/migrations/*new_cp15_step_definition_runtime_fields*.sql`

Plan:

- Replace current step-definition runtime fields with explicit fields:
  - `skills: string[]`
  - `subagent: string | null`
  - `model: string`
  - `teamRole: string | null` if retained from spec alignment
- Keep `requiredMcps`, input bindings, and output bindings as-is.
- Deprecate `requiredSkills` and `agentType` in the domain model after migration; if needed, map them as compatibility aliases during rollout only.
- Add a model dropdown in create/edit step-definition screens using a shared constant catalog grouped by family:
  - GPT / Codex-compatible
  - Gemini
  - Claude
- Keep skills editable as structured list input, not one freeform comma field long-term. For CP-15, comma-separated entry is acceptable only if parsed into canonical arrays before persistence.
- Add an optional `subagent` field with clear semantics separate from `skills`.

Schema direction:

- Add columns on `step_definitions` for `subagent`, `model`, and optional `team_role`.
- If `required_skills` already exists, either:
  - rename logically in code to `skills` while reusing the column, or
  - add `skills jsonb` and migrate existing values.

Recommendation:

- Reuse `required_skills` storage initially and map it to `skills` in code to reduce migration risk. Add only truly missing columns now.

### 4. Workflow start payload and runtime state

Objective: unify run creation around a payload that supports both full workflow and single-step starts, with begin prompt and step metadata available to runtime progression.

Files:

- `apps/admin-web/src/domain/model/payload/workflow-payload.ts`
- `apps/admin-web/src/domain/gateway/workflow-engine-gateway.ts`
- `apps/admin-web/src/domain/usecase/workflow-engine/start-workflow-run-usecase.ts`
- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts`
- `supabase/functions/workflow-engine-start-run/index.ts`
- `supabase/functions/_shared/workflow-engine-runtime.ts`
- `supabase/functions/_shared/workflow-engine-state-machine.ts`
- `supabase/migrations/*new_cp15_workflow_run_contract*.sql`

Plan:

- Expand `StartWorkflowRunPayload` to carry:
  - `projectId`
  - `workflowId`
  - `startMode`
  - `beginPrompt`
  - `stepType` or `workflowStepId` for single-step mode
- Update `WorkflowEngineGateway.startWorkflowRun()` to take the payload object, not positional ids.
- Persist begin prompt on the run or first run-step context. Recommendation:
  - add `begin_prompt` to `workflow_runs`
  - optionally add `resolved_input_markdown` on `workflow_run_steps` later if step-level caching becomes necessary
- For single-step mode, create a `workflow_run` plus exactly one executable `workflow_run_step`, without needing a synthetic one-step workflow definition.
- Extend runtime helpers so `progressWorkflowRun()` can load:
  - project workspace path
  - selected step definition runtime fields
  - required MCP list
  - input/output artifact bindings

Design note:

- Keep run creation in the edge function, not in the browser, because MCP gating, access control, and step materialization must stay server-authoritative.

### 5. MCP gating and project integration resolution

Objective: stop execution before the runner is called when required MCP dependencies are unavailable for the project.

Files:

- `supabase/functions/_shared/workflow-engine-runtime.ts`
- `apps/admin-web/src/features/mcp/integration-config.ts`
- related project integration queries in:
  - `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`
  - `supabase/migrations/*only_if_needed*`

Plan:

- Before marking a run-step runnable, resolve its `requiredMcps` against project-linked integrations and/or runner-visible MCP backends.
- Fail fast with user-visible guidance when a required MCP is missing or disconnected.
- Record the failure on the run-step and run with actionable error text:
  - missing MCP type
  - expected project integration
  - next action route
- Keep gating in the Supabase runtime, not the web UI, so retries and resumes behave consistently.

Recommendation:

- Treat “project has linked integration of required type and it is connected/enabled” as the primary gate.
- Treat raw runner backend presence alone as insufficient for project-scoped MCP requirements.

### 6. Artifact input/output handoff

Objective: let each step consume bound artifacts and write declared outputs after execution.

Files:

- `supabase/functions/_shared/workflow-engine-runtime.ts`
- `apps/local-runner/internal/runner/types.go`
- `apps/local-runner/internal/runner/runner.go`
- `apps/local-runner/internal/runner/artifacts.go`
- `apps/admin-web/src/domain/model/entity/local-runner.ts`
- `apps/admin-web/src/domain/gateway/local-runner-gateway.ts`
- `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`

Plan:

- In runtime progression, resolve input artifact bindings for the current step from prior `artifact_runs`.
- Build a structured runner request that includes:
  - run id / step id
  - project id
  - project working directory
  - begin prompt
  - resolved input artifact metadata and content paths
  - step definition skills / subagent / model / MCPs
  - declared output artifact definitions
- Extend runner artifact save logic so artifacts are no longer always hard-coded as `projectID=local` and `stepKey=prompt_execution`.
- Persist returned output artifacts back into `artifact_runs` tied to the real `workflow_run_step_id`.

Recommendation:

- Pass artifact references and file paths to the runner, not full artifact blobs, except where inline prompt assembly needs small markdown summaries.
- Let the runner write local output files first; let Supabase remain the system of record for artifact metadata.

### 7. Go runner prompt assembly and model-family routing

Objective: make the runner assemble execution input from step metadata and route to the correct CLI by model family.

Files:

- `apps/local-runner/internal/runner/types.go`
- `apps/local-runner/internal/runner/runner.go`
- `apps/local-runner/internal/runner/runner_test.go`
- optional new files:
  - `apps/local-runner/internal/runner/model_routing.go`
  - `apps/local-runner/internal/runner/prompt_assembly.go`

Plan:

- Replace the MVP `PromptExecutionRequest` with a workflow-oriented request shape that can still support simple prompt execution.
- Add explicit fields for:
  - `Model`
  - `BeginPrompt`
  - `Skills`
  - `Subagent`
  - `RequiredMcps`
  - `InputArtifacts`
  - `OutputArtifacts`
  - `ProjectID`
  - `WorkflowRunID`
  - `WorkflowRunStepID`
  - `WorkingDirectory`
- Add a small model-family resolver:
  - `resolveModelFamily(model string) -> codex|gemini|claude`
- Split provider execution into family-specific command builders instead of keeping one `codex` hard-check.
- Keep prompt assembly isolated from command execution:
  - prompt assembly builds canonical markdown/text
  - command builder chooses binary and args
- Ensure command execution always uses `WorkingDirectory` derived from the project path.

Recommendation:

- Implement this as an internal adapter table, not a switch spread across `ExecutePrompt()`. The likely stable boundary is:
  - validate request
  - assemble prompt
  - resolve model family
  - build command
  - execute
  - persist artifact bundle

### 8. Tests

Objective: cover the contract changes without broad UI snapshot churn.

Admin web tests:

- `apps/admin-web/src/routes/_authenticated/projects/$projectId.test.tsx`
- `apps/admin-web/src/routes/_authenticated/projects/create.test.tsx`
- `apps/admin-web/src/routes/_authenticated/workflow-steps/create.test.tsx`
- `apps/admin-web/src/routes/_authenticated/workflow-steps/$stepType.test.tsx`
- `apps/admin-web/src/domain/usecase/workflow-engine/workflow-engine-usecases.test.ts`
- `apps/admin-web/src/data/repository/supabase/workflow-engine-mappers.test.ts`
- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.test.ts`
- `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.test.ts`

Supabase runtime tests:

- `supabase/functions/_shared/workflow-engine-state-machine.test.ts`
- add focused tests for runtime helper logic around:
  - single-step start materialization
  - begin prompt persistence
  - MCP gating failure
  - artifact binding resolution

Runner tests:

- `apps/local-runner/internal/runner/runner_test.go`

Test focus:

- create-project rejects missing path
- project detail launcher enforces begin prompt and disables launch without path
- step definition save/load round-trips `skills`, `subagent`, `model`
- gateway start payload serializes new fields correctly
- start-run edge function creates either full run-step sequence or single-step run
- runtime blocks on missing MCP with clear error message
- runner maps `gpt-*`, `gemini-*`, `claude-*` to the correct command family
- runner uses requested working directory
- artifact outputs are recorded against the actual workflow run step

## Recommended Implementation Order

1. Schema and domain contract baseline
   - add project-path enforcement migration path
   - add step-definition runtime fields
   - add workflow-run start payload persistence fields
2. Admin-web model/gateway updates
   - project payload/domain updates
   - workflow-engine gateway and usecase signature updates
   - step-definition mapper/gateway updates
3. Project create/edit flow
   - required workspace selector
   - persist and validate `directoryPath`
4. Step-definition UI
   - model dropdown
   - skills/subagent fields
   - artifact binding continuity
5. Project detail launcher
   - 3-mode trigger UI
   - workflow and single-step submission
6. Supabase start-run and runtime logic
   - unified payload handling
   - single-step materialization
   - MCP gating
   - artifact input/output resolution
7. Go runner
   - request shape expansion
   - prompt assembly
   - model-family command routing
   - workflow-aware artifact persistence
8. Tests and cleanup
   - update failing tests
   - remove stale `agentType` / positional start-run assumptions

## Risks and Controls

- Legacy null `directory_path` rows can block hard non-null rollout.
  - Control: two-step migration with manual cleanup query.
- Existing workflow-level provider/model overrides can conflict with step-level model.
  - Control: define precedence explicitly: step model > workflow override > unsupported/null.
- Runner adapter behavior may differ across Codex, Gemini, and Claude CLIs.
  - Control: isolate family-specific command builders and test command construction without requiring all CLIs in CI.
- Artifact linkage can drift if runtime and runner both invent ids.
  - Control: edge runtime remains the source of ids; runner receives ids and must echo metadata back.

## Explicit Non-Goals

- No redesign of workflow builder UX beyond the project-detail trigger integration.
- No full prompt-template system beyond the minimum runtime assembly needed for CP-15.
- No provider-specific optimization layer beyond model-family routing.
- No TDD signature document generation in this phase.

## Delivery Note

GitNexus impact-analysis tools were requested by repo instructions but are not available in this session. This plan is based on direct code inspection of the current admin-web, Supabase runtime, and local-runner surfaces.
