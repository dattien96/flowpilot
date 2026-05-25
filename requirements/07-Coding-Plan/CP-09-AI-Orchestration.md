# CP-09: AI Orchestration - Provider Resolution, Prompt Templates & Execution Logs

**Maps from:** SS-05 (AI Provider), SS-06 (Skill Agent), SS-07 (Artifacts), SD-06 (AI Provider Integration), SD-07 (Skill/Agent Runtime), SD-08 (Artifact Management), SD-10 (Context Resolver RAG)
**Phase:** 6
**Depends on:** CP-07, CP-06, CP-08

---

## 1. Core Concept

This phase wires the AI execution pipeline on top of the verified workflow and artifact model:

1. Provider inventory, install/auth flows, and provider-model-reasoning resolution shared with the workflow engine.
2. AI prompt template registry for reusable step-oriented prompts.
3. Browser-triggered generation through the canonical workflow engine, with direct Edge Function contracts kept as the target API surface for future browser-to-AI flows.
4. AI execution logs for call-level auditability.
5. Artifact management for generated outputs, versioning, and annotations.
6. Embedding support via a shared `generate-embedding` Edge Function.

Important product rule:

- Any durable generation triggered from the UI must still produce canonical runtime lineage.
- That means the generation flow should create or reuse a real `workflow_run` and `workflow_run_step`, then write `ai_runs` and `artifact_runs`.
- CP-09 is not allowed to reintroduce a standalone artifact runtime path outside the CP-06 / CP-07 model.
- Current implementation routes the "Generate from..." buttons through `GenerationLaunchPanel` and the workflow engine launch flow, not direct `generate-*` Edge Functions.

### 1.1 Provider & Model Orchestration Foundation

This phase must implement the missing execution-planning work defined in `SD-06-AI-Provider-Integration.md`, not just prompt/template storage.

Required implementation tracks:

1. Add machine-readable provider inventory for the desktop app via `flowpilot providers list --json`.
2. Add provider installation entrypoint via `flowpilot install-provider <provider_name>`.
3. Add project/workflow/step/run persistence for provider/model/reasoning resolution.
4. Add settings and workflow-launch UI for selecting model/reasoning defaults and overrides, with provider derived automatically.
5. Add shared validation so provider-model-reasoning combinations are checked before durable execution starts.
6. Keep host-machine install/auth state out of `projects` and return it from the runner inventory API instead.
7. Proxy provider install requests from Admin Web through `/api/local-runner/providers/install` to the runner `/providers/install` HTTP endpoint.

### 1.2 Provider Scope Boundaries

This phase must explicitly separate two concerns:

- Project configuration:
  - `projects.default_model`
  - `projects.default_reasoning_effort`
  - `workflows.model_override`
  - `workflows.reasoning_effort_override`
  - `workflow_steps.model_override`
  - `workflow_steps.reasoning_effort_override`
  - `workflow_runs.model`
  - `workflow_runs.reasoning_effort`
- `projects.default_provider`, `workflows.provider_override`, `workflow_steps.provider_override`, and `workflow_runs.provider` as derived reference fields only
- Host-machine runtime state:
  - install status
  - auth readiness
  - detected binary path
  - detected CLI version

Host-machine runtime state belongs to the Go-Runner provider inventory contract and must not be persisted as project settings.

---

## 2. Database Tables

### 2.1 Provider Orchestration Persistence

Before implementing prompt-template and generation flows, the canonical workflow schema must persist provider/model/reasoning configuration in the same places defined by `SD-06`.

```sql
ALTER TABLE projects
  ADD COLUMN default_provider TEXT,
  ADD COLUMN default_model TEXT,
  ADD COLUMN default_reasoning_effort TEXT;

ALTER TABLE workflows
  ADD COLUMN IF NOT EXISTS provider_override TEXT,
  ADD COLUMN IF NOT EXISTS model_override TEXT,
  ADD COLUMN IF NOT EXISTS reasoning_effort_override TEXT;

ALTER TABLE workflow_steps
  ADD COLUMN IF NOT EXISTS provider_override TEXT,
  ADD COLUMN IF NOT EXISTS model_override TEXT,
  ADD COLUMN IF NOT EXISTS reasoning_effort_override TEXT;

ALTER TABLE workflow_runs
  ADD COLUMN IF NOT EXISTS provider TEXT,
  ADD COLUMN IF NOT EXISTS model TEXT,
  ADD COLUMN IF NOT EXISTS reasoning_effort TEXT;

ALTER TABLE ai_runs
  ADD COLUMN IF NOT EXISTS provider TEXT,
  ADD COLUMN IF NOT EXISTS reasoning_effort TEXT;
```

Rules:

- Store provider IDs exactly as the product identifiers from `SD-06`: `CLAUDE`, `CODEX`, `GEMINI`.
- Store model IDs verbatim as provider-native strings such as `gpt-5.5` or `claude-sonnet-4-20250514`.
- Store reasoning effort values as `low`, `medium`, `high`, `xhigh` or NULL.
- `workflow_runs.provider`, `workflow_runs.model` and `workflow_runs.reasoning_effort` are the resolved run-level baseline captured when the run starts, with provider derived from the resolved model.
- `ai_runs.provider`, `ai_runs.model_name` and `ai_runs.reasoning_effort` are the actual values used for that individual AI call.
- If `workflow_steps` already contains override columns, Phase 6 work must reuse them instead of creating parallel fields.

Resolution contract to implement:

1. Step override wins: `workflow_steps.model_override` / `workflow_steps.reasoning_effort_override`
2. Run baseline next: `workflow_runs.model` / `workflow_runs.reasoning_effort`
3. Project fallback last: `projects.default_model` / `projects.default_reasoning_effort`
4. Final defaults: `gpt-5.4` and `medium`

Workflow-level overrides are applied when the run is created and provider is derived from the resolved model:

```text
runModel = launchOverride.model
    ?? workflow.model_override
    ?? project.default_model
    ?? "gpt-5.4"

runReasoning = launchOverride.reasoning_effort
    ?? workflow.reasoning_effort_override
    ?? project.default_reasoning_effort
    ?? "medium"

stepModel = step.model_override ?? runModel
stepReasoning = step.reasoning_effort_override ?? runReasoning
stepProvider = providerFrom(stepModel)
```

Validation tasks:

- reject run creation if model/reasoning cannot be resolved
- reject run creation if the resolved model is not supported
- reject step execution if a step override produces an invalid model/reasoning combination

### 2.2 AI Prompt Templates

Prompt templates are reusable prompt bodies for workflow step types and browser-triggered generation flows.

```sql
CREATE TABLE ai_prompt_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID REFERENCES projects(id) ON DELETE CASCADE, -- NULL = global template
  step_type TEXT NOT NULL,                                   -- maps to workflow step types
  name TEXT NOT NULL,
  description TEXT,
  input_schema JSONB DEFAULT '{}',
  output_schema JSONB DEFAULT '{}',
  template_content TEXT NOT NULL,
  provider_preference TEXT,                                  -- claude, codex, gemini, or NULL
  model_preference TEXT,
  version INT NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'active',                     -- active, archived
  created_by UUID REFERENCES auth.users(id),
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE UNIQUE INDEX ai_prompt_templates_scope_step_version_uniq
  ON ai_prompt_templates (
    COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid),
    step_type,
    version
  );

ALTER TABLE ai_prompt_templates ENABLE ROW LEVEL SECURITY;

CREATE POLICY "ai_prompt_templates_select_authenticated"
  ON ai_prompt_templates FOR SELECT
  USING (
    project_id IS NULL
    OR project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );

CREATE POLICY "ai_prompt_templates_insert_authenticated"
  ON ai_prompt_templates FOR INSERT
  WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "ai_prompt_templates_update_project_members"
  ON ai_prompt_templates FOR UPDATE
  USING (
    project_id IS NOT NULL
    AND project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  )
  WITH CHECK (
    project_id IS NOT NULL
    AND project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );

CREATE POLICY "ai_prompt_templates_delete_project_members"
  ON ai_prompt_templates FOR DELETE
  USING (
    project_id IS NOT NULL
    AND project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );
```

Version history should be modeled explicitly. For MVP, append-only rows with incremented `version` are acceptable, but the UI must show prior versions from the table history, not from a fake in-row audit field. Template resolution should be deterministic:

- project-specific templates override global templates for the same `step_type`
- within a scope, the highest active `version` wins by default

### 2.3 AI Runs

`ai_runs` is a call-level audit table. It records each LLM/API execution, including retries and sub-calls, but it is not the owner of workflow state.

```sql
CREATE TABLE ai_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  run_type TEXT NOT NULL,                  -- tech_spec_generation, coding_plan_generation, schedule_generation, business_review
  input_payload JSONB NOT NULL,
  output_payload JSONB,
  provider TEXT NOT NULL,
  model_name TEXT NOT NULL,
  triggered_by UUID NOT NULL REFERENCES auth.users(id),
  status TEXT NOT NULL DEFAULT 'running',  -- running, success, failed
  error_message TEXT,
  tokens_input INT,
  tokens_output INT,
  cost_usd NUMERIC(10,6),
  prompt_template_id UUID REFERENCES ai_prompt_templates(id),
  workflow_run_id UUID REFERENCES workflow_runs(id) ON DELETE SET NULL,
  workflow_run_step_id UUID REFERENCES workflow_run_steps(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  completed_at TIMESTAMPTZ,
  CHECK (
    (workflow_run_id IS NULL AND workflow_run_step_id IS NULL)
    OR
    (workflow_run_id IS NOT NULL AND workflow_run_step_id IS NOT NULL)
  )
);

ALTER TABLE ai_runs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "ai_runs_select_project_members"
  ON ai_runs FOR SELECT
  USING (
    project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );

CREATE POLICY "ai_runs_insert_authenticated"
  ON ai_runs FOR INSERT
  WITH CHECK (auth.role() = 'authenticated');

CREATE POLICY "ai_runs_update_project_members"
  ON ai_runs FOR UPDATE
  USING (
    project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  )
  WITH CHECK (
    project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );
```

Rule:

- Workflow-triggered generation must populate both `workflow_run_id` and `workflow_run_step_id`.
- If a browser-triggered action is persisted, it should still be executed as a single-step workflow run so the lineage stays canonical.
- Nullable lineage is allowed only for ephemeral or diagnostic calls that do not create durable artifacts.
- `provider` + `model_name` must reflect the actual call values after override resolution, not only prompt-template preference values.

### 2.4 Artifact Annotations

Annotations belong to the canonical `artifact_runs` table, not a legacy `artifacts` table.

```sql
CREATE TABLE artifact_annotations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  artifact_run_id UUID NOT NULL REFERENCES artifact_runs(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES auth.users(id),
  highlight_range TEXT,                  -- e.g. "L15-L22"
  note_text TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE artifact_annotations ENABLE ROW LEVEL SECURITY;

CREATE POLICY "artifact_annotations_select"
  ON artifact_annotations FOR SELECT
  USING (
    artifact_run_id IN (
      SELECT ar.id FROM artifact_runs ar
      WHERE ar.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  );

CREATE POLICY "artifact_annotations_insert"
  ON artifact_annotations FOR INSERT
  WITH CHECK (
    artifact_run_id IN (
      SELECT ar.id FROM artifact_runs ar
      WHERE ar.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  );

CREATE POLICY "artifact_annotations_update"
  ON artifact_annotations FOR UPDATE
  USING (
    artifact_run_id IN (
      SELECT ar.id FROM artifact_runs ar
      WHERE ar.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  )
  WITH CHECK (
    artifact_run_id IN (
      SELECT ar.id FROM artifact_runs ar
      WHERE ar.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  );

CREATE POLICY "artifact_annotations_delete"
  ON artifact_annotations FOR DELETE
  USING (
    artifact_run_id IN (
      SELECT ar.id FROM artifact_runs ar
      WHERE ar.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  );
```

---

## 3. Supabase Edge Functions & Runner Contracts

### 3.1 Go-Runner Provider Inventory Commands

This phase must include the desktop-app runner contract required by `SD-06`.

```text
flowpilot providers list --json
flowpilot install-provider <provider_name>
```

Implementation tasks:

1. Detect installed CLIs using `exec.LookPath(...)`.
2. Resolve version using `<provider-binary> --version`.
3. Return provider inventory with `supported`, `install_status`, `auth_status`, `detected_binary`, `detected_version`, `models`, and `last_error`.
4. Add OS-aware install branches for `CLAUDE`, `CODEX`, and `GEMINI`.
5. Re-run verification after install and refresh inventory before returning control to the UI.
6. Expose model lists through provider inventory using runtime discovery when available, otherwise bundled FlowPilot registry fallback.
7. Return the inventory fields that the admin UI consumes today, including provider key, display label, install status, auth status, detected binary, detected version, models, and last error.

The frontend settings page must treat `flowpilot providers list --json` as the source of truth for install/auth state.

### 3.2 `generate-tech-spec`

The following direct Edge Function contracts are the target browser-triggered API surface. The current repository implementation wires the UI through canonical workflow launches instead.

```text
POST /functions/v1/generate-tech-spec

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  promptTemplateId?: string,
  modelOverride?: string,
  reasoningEffortOverride?: "low" | "medium" | "high" | "xhigh"
}

Response: {
  aiRunId: string,
  workflowRunId: string,
  workflowRunStepId: string,
  artifactRunId: string,
  content: string
}
```

### 3.3 `generate-coding-plan`

```text
POST /functions/v1/generate-coding-plan

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  promptTemplateId?: string,
  modelOverride?: string,
  reasoningEffortOverride?: "low" | "medium" | "high" | "xhigh"
}

Response: {
  aiRunId: string,
  workflowRunId: string,
  workflowRunStepId: string,
  artifactRunId: string,
  content: string
}
```

### 3.4 `review-business-logic`

```text
POST /functions/v1/review-business-logic

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  promptTemplateId?: string,
  modelOverride?: string,
  reasoningEffortOverride?: "low" | "medium" | "high" | "xhigh"
}

Response: {
  aiRunId: string,
  workflowRunId: string,
  workflowRunStepId: string,
  reviewNotes: string,
  ambiguities: string[],
  artifactRunId?: string
}
```

### 3.5 `generate-master-schedule`

```text
POST /functions/v1/generate-master-schedule

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  teamMembers: TeamMember[],
  sprintLength?: number,
  deadlines?: { name: string, date: string }[],
  promptTemplateId?: string,
  modelOverride?: string,
  reasoningEffortOverride?: "low" | "medium" | "high" | "xhigh"
}

Response: {
  aiRunId: string,
  workflowRunId: string,
  workflowRunStepId: string,
  masterScheduleId: string,
  scheduleItems: ScheduleItem[]
}
```

### 3.6 Edge Function Pattern

All AI Edge Functions follow the same pattern:

1. Validate auth with the Supabase Auth JWT.
2. Resolve project access and the source `artifact_run`.
3. Resolve the prompt template or use the default template for the step type.
4. Resolve Model and Reasoning using the same precedence rules as the Go-Runner.
5. Derive Provider from the resolved model and validate that the resolved model belongs to that provider and that the reasoning effort is valid.
6. Create or reuse the canonical workflow run and workflow run step for this generation.
7. Assemble the prompt.
8. Call the AI provider API.
9. Insert `ai_runs` with the actual `provider` and `model_name`.
10. Write the generated output to Supabase Storage, then insert `artifact_runs` with `remote_path` and `remote_url`.
11. Update `workflow_run_steps.status`.
12. Return `aiRunId`, `workflowRunId`, `workflowRunStepId`, and `artifactRunId`.

Provider-resolution note:

- Edge Functions do not perform host-machine CLI installation.
- They must still reuse the same provider IDs, model/reasoning validation rules, and run/step override semantics as the Go-Runner path.

Important storage rule:

- Edge Functions do not have local filesystem access.
- They create storage-backed artifact runs, not local-first `.artifacts` files.
- Go-Runner remains the local-first execution path for workflow runs.
- Storage convention for browser-triggered artifact generation:
  - Bucket: `flowpilot-artifacts`
  - Path: `{projectId}/{workflowRunId}/{workflowRunStepId}/{artifactKey}/{version}/{fileName}`

```typescript
// supabase/functions/generate-tech-spec/index.ts
import { serve } from 'https://deno.land/std@0.177.0/http/server.ts'
import { createClient } from 'https://esm.sh/@supabase/supabase-js@2'

serve(async (req) => {
  const supabase = createClient(
    Deno.env.get('SUPABASE_URL')!,
    Deno.env.get('SUPABASE_SERVICE_ROLE_KEY')!
  )

  const authHeader = req.headers.get('Authorization')
  if (!authHeader) {
    return new Response(JSON.stringify({ error: 'Missing auth header' }), { status: 401 })
  }

  const { data: { user } } = await supabase.auth.getUser(authHeader.replace('Bearer ', ''))
  if (!user) {
    return new Response(JSON.stringify({ error: 'Unauthorized' }), { status: 401 })
  }

  // Resolve source artifact_run, create workflow lineage, call provider, persist ai_runs + artifact_runs.
})
```

### 3.7 `generate-embedding`

This shared Edge Function supports artifact memory search in CP-12.

```text
POST /functions/v1/generate-embedding

Body: {
  text: string
}

Response: {
  embedding: number[],
  dimensions: number
}
```

Implementation requirements:

- validate method and input
- require JWT auth to prevent unauthenticated use
- support CORS preflight
- use `new Supabase.ai.Session("gte-small")`
- call `session.run(text, { mean_pool: true, normalize: true })`
- return `dimensions` for debugging and migration validation

`generate-embedding` is a shared utility function. Project access control should be enforced by the caller before sending text to this function. The CORS allowlist is not the security boundary. JWT validation is.

---

## 4. UI Components

### 4.1 Provider Settings (`/settings/ai-providers`)

- Show the three supported providers: `CLAUDE`, `CODEX`, `GEMINI`.
- Load provider inventory from `flowpilot providers list --json`.
- Show per-provider:
  - install status
  - auth status
  - detected version
  - selectable models
- Persist `projects.default_provider`, `projects.default_model`, and `projects.default_reasoning_effort`.
- Actions:
  - `Refresh Detection`
  - `Install`
  - `Authenticate` only when the selected provider exposes an explicit auth flow; otherwise surface auth readiness and the next required host-side step.
- Validation:
  - model dropdown filtered by selected provider
  - block save when selected model does not belong to selected provider
  - validate selected reasoning effort level if applicable

### 4.2 Workflow Builder / Launch Overrides

- Workflow builder must expose `workflows.model_override` and `workflows.reasoning_effort_override`; provider is shown read-only as a derived badge.
- Step editor must expose `workflow_steps.model_override` and `workflow_steps.reasoning_effort_override`; provider is shown read-only as a derived badge.
- Workflow launch UI must allow run-time override before a `workflow_run` is created.
- Launch screen must warn when the chosen provider is not installed or still requires auth.
- Inherited values must be visible in the UI so users can tell whether a step is using step, workflow, run, or project defaults.

### 4.3 AI Prompt Templates (`/settings/prompt-templates`)

- DataTable: Name, Step Type, Scope, Provider Preference, Model Preference, Version, Status
- Create/Edit dialog:
  - Name, Description
  - Step Type
  - Scope selector: global or project
  - Template Content editor with placeholder syntax
  - Input/Output Schema JSON editor
- Provider Preference and Model Preference
- Version history: show prior versions from the template history rows
- Template preference is advisory metadata for prompt selection and must not replace workflow/run/step provider resolution.

### 4.4 AI Execution Logs (`/ai-runs`)

- DataTable: Run Type, Project, Provider, Model, Reasoning Effort, Status, Tokens, Cost, Triggered By, Date
- Click row -> detail view:
  - Full input payload
  - Full output payload
  - Resolved provider/model/reasoning used
  - Token usage breakdown
  - Cost estimate
- Linked workflow run and workflow step
- Linked artifact run
- Filter by project, provider, status, model, reasoning effort, date range
- Retry/Regenerate creates a new single-step `workflow_run` with new lineage, not an in-place mutation
- This is different from the CP-07 reject/retry loop, which reuses the existing `workflow_run_step` and increments `retry_count`

### 4.5 Artifact Viewer

- Markdown rendering of artifact content loaded from `artifact_runs.remote_url`
- Version history sidebar
- Annotations saved to `artifact_annotations`
- Compare versions side-by-side

### 4.6 AI Generation Trigger Buttons

Wire the previously-disabled "Generate from..." buttons. In the current implementation these are launch panels that create canonical single-step workflow runs:

- Business Logic page -> `business_summary`
- Tech Spec page -> `tech_spec`
- Coding Plan page -> `make_plan_coding`
- Master Schedule page -> `task_breakdown`

All calls go through `GenerationLaunchPanel` in the app layer, require a valid project workspace binding, and return the canonical workflow lineage plus the created `artifact_run`.

---

## 5. Go-Runner <-> Admin Web Communication

For workflow execution, the Go-Runner drives the pipeline:

- Admin Web calls `flowpilot providers list --json` to render provider inventory and models.
- Admin Web calls `flowpilot install-provider <provider_name>` when the user chooses install from settings, via the local-runner proxy route at `/api/local-runner/providers/install`.
- The Go-Runner polls `workflow_run_steps` for `PENDING` rows.
- Before claiming or executing a step, the runner resolves Provider/Model/Reasoning from project/workflow/run/step configuration.
- The runner validates the provider/model/reasoning combination and confirms install/auth readiness for the resolved provider.
- It calls the AI provider directly.
- It writes results back to `artifact_runs`, writes `ai_runs`, and updates `workflow_run_steps.status`.
- The Admin Web receives updates via Supabase Realtime on `workflow_run_steps`.

For browser-triggered AI generation, the Admin Web calls Edge Functions directly:

- Edge Functions handle the AI call securely.
- They still create workflow lineage, `ai_runs`, and `artifact_runs`.
- They must reuse the same provider IDs, override semantics, and provider-model-reasoning validation rules as the runner path.

This keeps the workflow engine independent from the admin UI while preserving one canonical runtime model.

---

## 6. Definition of Done - Phase 6

### Database

- `projects.default_provider`, `projects.default_model`, and `projects.default_reasoning_effort` exist and are used as the project baseline.
- `workflows.provider_override` / `model_override` / `reasoning_effort_override` and `workflow_steps.provider_override` / `model_override` / `reasoning_effort_override` are populated through derived-provider resolution logic.
- `workflow_runs.provider`, `workflow_runs.model`, and `workflow_runs.reasoning_effort` are populated when runs start.
- `ai_prompt_templates` with step type, scope, versioning, and RLS SELECT/INSERT/UPDATE/DELETE policies.
- `ai_runs` with project-scoped audit fields, `provider`, `model_name`, `reasoning_effort`, `workflow_run_id`, `workflow_run_step_id`, token/cost columns, and RLS project-member policy.
- `artifact_annotations` with `artifact_run_id UUID REFERENCES artifact_runs(id)` and project-member RLS.

### Edge Functions

- `flowpilot providers list --json` returns install/auth/model inventory for `CLAUDE`, `CODEX`, and `GEMINI`.
- `flowpilot install-provider <provider_name>` performs OS-aware install + post-install verification.
- `generate-tech-spec`, `generate-coding-plan`, `generate-master-schedule`, and `review-business-logic` remain the target direct Edge Function contracts for future browser-triggered generation.
- `generate-embedding` is shared and returns `{ embedding, dimensions }` for CP-12 RAG.
- Browser-triggered generation endpoints accept optional `modelOverride` and `reasoningEffortOverride`; provider is derived from the resolved model.
- All durable generation paths enforce provider-model-reasoning validation before execution.

### UI

- Provider settings page shows provider inventory, install/auth actions, and project default Model/Reasoning selectors plus a derived provider badge.
- Workflow builder and workflow launch UI expose model/reasoning overrides with derived-provider visibility.
- Prompt Template CRUD with markdown editor, schema editors, and version history.
- AI Execution Log viewer with provider, model, reasoning_effort, tokens, cost, status filtering, and detail drill-down.
- All "Generate from..." buttons call the correct Edge Functions with artifact-run inputs.
- Artifact viewer with version history and annotation highlighting.
- Annotation create/edit/delete scoped to project members.

### Integration

- Provider resolution order matches `SD-06`: step model -> run model -> project model -> default model, with provider derived from the resolved model.
- Workflow-level overrides are applied when the run is created.
- Go-Runner polls `workflow_run_steps` for `PENDING` rows.
- Go-Runner checks provider install/auth readiness before step execution.
- Go-Runner writes results to `artifact_runs`.
- Admin Web reads workflow progress via Supabase Realtime on `workflow_run_steps`.
- Every durable AI output has traceable workflow lineage and a persisted artifact run.

---

## 7. Manual Verification Guide

Use this checklist after implementation. The main invariant is:

`workflow_run` -> `workflow_run_step` -> `ai_runs` -> `artifact_runs`

Every durable generation should leave this lineage behind.

### 7.1 Database Verification

Run one simple flow first, such as Business Logic -> Generate Tech Spec. Confirm:

- one new `workflow_run` was created
- one new `workflow_run_step` was created for the generation step
- one `ai_runs` row points to that `workflow_run` and `workflow_run_step`
- one `artifact_runs` row points to that same `workflow_run` and `workflow_run_step`
- `workflow_run_steps.status` ends in `DONE` or `WAITING_USER_APPROVAL`
- Edge Function-generated artifacts have `remote_url` populated

Suggested SQL:

```sql
select * from workflow_runs order by started_at desc limit 5;
select * from workflow_run_steps order by started_at desc limit 10;
select * from ai_runs order by created_at desc limit 10;
select * from artifact_runs order by created_at desc limit 10;
select id, default_provider, default_model from projects order by updated_at desc limit 10;
select id, provider_override, model_override from workflows order by updated_at desc limit 10;
select id, workflow_id, provider_override, model_override from workflow_steps order by updated_at desc limit 20;
```

### 7.2 Edge Function Verification

For the target direct Edge Function surface, if and when it is enabled, call `generate-tech-spec` directly with a valid `sourceArtifactRunId`. Confirm:

- response returns `workflowRunId`, `workflowRunStepId`, and `artifactRunId`
- response respects optional `modelOverride` and `reasoningEffortOverride`
- invalid JWT returns `401`
- invalid or missing `sourceArtifactRunId` returns `400` or `404`
- unauthorized project access is rejected
- invalid provider/model/reasoning combinations are rejected before the AI call
- repeated regenerate calls create a fresh single-step workflow lineage

For the current implementation, verify the same lineage through the Admin Web launch panels instead of calling direct `generate-*` endpoints.

### 7.3 Go-Runner Verification

Execute one real workflow through the Go-Runner. Confirm:

- `flowpilot providers list --json` returns install/auth state and models
- install flow can move a provider from `NOT_INSTALLED` to `INSTALLED` after verification
- the runner claims `PENDING` `workflow_run_steps`
- `workflow_runs.provider` and `workflow_runs.model` are populated from the correct precedence chain
- a step-level override changes only that step's execution model/reasoning, and provider follows from the model
- the output file is written to the local path resolved from `artifact_definitions`
- sync populates `artifact_runs.remote_path` and `artifact_runs.remote_url`
- the step points to the produced `artifact_run`
- `ai_runs.provider` and `ai_runs.model_name` match the actual execution
- retry behavior follows the workflow engine contract for runner-owned steps

### 7.4 UI Verification

From Admin Web, confirm:

- `/settings/ai-providers` lists supported providers, install status, auth status, and models
- saving project defaults persists to `projects.default_model` / `projects.default_reasoning_effort`, and the provider badge updates automatically from the model
- workflow builder shows workflow and step model/reasoning overrides with derived provider display
- `Generate from ...` buttons create real lineage-backed outputs
- `/settings/prompt-templates` can list and save templates
- `/ai-runs` shows provider, model, reasoning_effort, status, tokens, cost, and lineage ids
- artifact viewer opens the generated artifact version
- approval-required steps wait for user approval instead of silently completing

### 7.5 Minimum Regression Matrix

Run these cases before closing the feature:

1. Happy-path generation with a valid source artifact run
2. Invalid source artifact run id
3. Unauthorized user or missing JWT
4. Provider installed but auth still required
5. Invalid model for selected provider
6. Step override differs from workflow/run/project default (for both model and reasoning effort)
7. Approval-required generation step
8. Retry or regenerate from AI logs
9. Artifact save succeeds even if downstream memory extraction fails
10. Verify reasoning effort is correctly passed to underlying CLI tools (Codex & Claude Code)
