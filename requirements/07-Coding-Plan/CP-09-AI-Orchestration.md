# CP-09: AI Orchestration - Prompt Templates & Execution Logs

**Maps from:** SS-05 (AI Provider), SS-06 (Skill Agent), SS-07 (Artifacts), SD-06 (AI Provider Integration), SD-07 (Skill/Agent Runtime), SD-08 (Artifact Management), SD-10 (Context Resolver RAG)
**Phase:** 6
**Depends on:** CP-07, CP-06, CP-08

---

## 1. Core Concept

This phase wires the AI execution pipeline on top of the verified workflow and artifact model:

1. AI prompt template registry for reusable step-oriented prompts.
2. Supabase Edge Functions for secure browser-triggered generation.
3. AI execution logs for call-level auditability.
4. Artifact management for generated outputs, versioning, and annotations.
5. Embedding support via a shared `generate-embedding` Edge Function.

Important product rule:

- Any durable generation triggered from the UI must still produce canonical runtime lineage.
- That means the generation flow should create or reuse a real `workflow_run` and `workflow_run_step`, then write `ai_runs` and `artifact_runs`.
- CP-09 is not allowed to reintroduce a standalone artifact runtime path outside the CP-06 / CP-07 model.

---

## 2. Database Tables

### 2.1 AI Prompt Templates

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

### 2.2 AI Runs

`ai_runs` is a call-level audit table. It records each LLM/API execution, including retries and sub-calls, but it is not the owner of workflow state.

```sql
CREATE TABLE ai_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  run_type TEXT NOT NULL,                  -- tech_spec_generation, coding_plan_generation, schedule_generation, business_review
  input_payload JSONB NOT NULL,
  output_payload JSONB,
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

### 2.3 Artifact Annotations

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

## 3. Supabase Edge Functions

### 3.1 `generate-tech-spec`

```text
POST /functions/v1/generate-tech-spec

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  promptTemplateId?: string,
  modelOverride?: string
}

Response: {
  aiRunId: string,
  workflowRunId: string,
  workflowRunStepId: string,
  artifactRunId: string,
  content: string
}
```

### 3.2 `generate-coding-plan`

```text
POST /functions/v1/generate-coding-plan

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  promptTemplateId?: string,
  modelOverride?: string
}

Response: {
  aiRunId: string,
  workflowRunId: string,
  workflowRunStepId: string,
  artifactRunId: string,
  content: string
}
```

### 3.3 `review-business-logic`

```text
POST /functions/v1/review-business-logic

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  promptTemplateId?: string,
  modelOverride?: string
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

### 3.4 `generate-master-schedule`

```text
POST /functions/v1/generate-master-schedule

Body: {
  projectId: string,
  sourceArtifactRunId: string,
  teamMembers: TeamMember[],
  sprintLength?: number,
  deadlines?: { name: string, date: string }[],
  promptTemplateId?: string,
  modelOverride?: string
}

Response: {
  aiRunId: string,
  workflowRunId: string,
  workflowRunStepId: string,
  masterScheduleId: string,
  scheduleItems: ScheduleItem[]
}
```

### 3.5 Edge Function Pattern

All AI Edge Functions follow the same pattern:

1. Validate auth with the Supabase Auth JWT.
2. Resolve project access and the source `artifact_run`.
3. Resolve the prompt template or use the default template for the step type.
4. Create or reuse the canonical workflow run and workflow run step for this generation.
5. Assemble the prompt.
6. Call the AI provider API.
7. Insert `ai_runs`.
8. Write the generated output to Supabase Storage, then insert `artifact_runs` with `remote_path` and `remote_url`.
9. Update `workflow_run_steps.status`.
10. Return `aiRunId`, `workflowRunId`, `workflowRunStepId`, and `artifactRunId`.

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

### 3.6 `generate-embedding`

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

### 4.1 AI Prompt Templates (`/settings/prompt-templates`)

- DataTable: Name, Step Type, Scope, Provider Preference, Model Preference, Version, Status
- Create/Edit dialog:
  - Name, Description
  - Step Type
  - Scope selector: global or project
  - Template Content editor with placeholder syntax
  - Input/Output Schema JSON editor
  - Provider Preference and Model Preference
- Version history: show prior versions from the template history rows

### 4.2 AI Execution Logs (`/ai-runs`)

- DataTable: Run Type, Project, Model, Status, Tokens, Cost, Triggered By, Date
- Click row -> detail view:
  - Full input payload
  - Full output payload
  - Token usage breakdown
  - Cost estimate
- Linked workflow run and workflow step
- Linked artifact run
- Filter by project, status, model, date range
- Retry/Regenerate creates a new single-step `workflow_run` with new lineage, not an in-place mutation
- This is different from the CP-07 reject/retry loop, which reuses the existing `workflow_run_step` and increments `retry_count`

### 4.3 Artifact Viewer

- Markdown rendering of artifact content loaded from `artifact_runs.remote_url`
- Version history sidebar
- Annotations saved to `artifact_annotations`
- Compare versions side-by-side

### 4.4 AI Generation Trigger Buttons

Wire the previously-disabled "Generate from..." buttons:

- Business Logic page -> "Ask AI to Review" calls `review-business-logic`
- Tech Spec page -> "Generate from Business Logic" calls `generate-tech-spec`
- Coding Plan page -> "Generate from Tech Spec" calls `generate-coding-plan`
- Master Schedule page -> "Generate Schedule" calls `generate-master-schedule`

All calls go through typed mutations in the app layer, and each generation flow must return the canonical workflow lineage plus the created `artifact_run`.

---

## 5. Go-Runner <-> Admin Web Communication

For workflow execution, the Go-Runner drives the pipeline:

- The Go-Runner polls `workflow_run_steps` for `PENDING` rows.
- It calls the AI provider directly.
- It writes results back to `artifact_runs` and updates `workflow_run_steps.status`.
- The Admin Web receives updates via Supabase Realtime on `workflow_run_steps`.

For browser-triggered AI generation, the Admin Web calls Edge Functions directly:

- Edge Functions handle the AI call securely.
- They still create workflow lineage, `ai_runs`, and `artifact_runs`.

This keeps the workflow engine independent from the admin UI while preserving one canonical runtime model.

---

## 6. Definition of Done - Phase 6

### Database

- `ai_prompt_templates` with step type, scope, versioning, and RLS SELECT/INSERT/UPDATE/DELETE policies.
- `ai_runs` with project-scoped audit fields, `workflow_run_id`, `workflow_run_step_id`, token/cost columns, and RLS project-member policy.
- `artifact_annotations` with `artifact_run_id UUID REFERENCES artifact_runs(id)` and project-member RLS.

### Edge Functions

- `generate-tech-spec` accepts `sourceArtifactRunId` and writes `ai_runs` + `artifact_runs`.
- `generate-coding-plan` accepts `sourceArtifactRunId` and writes `ai_runs` + `artifact_runs`.
- `generate-master-schedule` accepts `sourceArtifactRunId` and writes the schedule row(s) plus `artifact_runs`.
- `review-business-logic` accepts `sourceArtifactRunId` and returns review notes and ambiguities.
- `generate-embedding` is shared and returns `{ embedding, dimensions }` for CP-12 RAG.

### UI

- Prompt Template CRUD with markdown editor, schema editors, and version history.
- AI Execution Log viewer with tokens, cost, status filtering, and detail drill-down.
- All "Generate from..." buttons call the correct Edge Functions with artifact-run inputs.
- Artifact viewer with version history and annotation highlighting.
- Annotation create/edit/delete scoped to project members.

### Integration

- Go-Runner polls `workflow_run_steps` for `PENDING` rows.
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
```

### 7.2 Edge Function Verification

Call `generate-tech-spec` directly with a valid `sourceArtifactRunId`. Confirm:

- response returns `workflowRunId`, `workflowRunStepId`, and `artifactRunId`
- invalid JWT returns `401`
- invalid or missing `sourceArtifactRunId` returns `400` or `404`
- unauthorized project access is rejected
- repeated regenerate calls create a fresh single-step workflow lineage

### 7.3 Go-Runner Verification

Execute one real workflow through the Go-Runner. Confirm:

- the runner claims `PENDING` `workflow_run_steps`
- the output file is written to the local path resolved from `artifact_definitions`
- sync populates `artifact_runs.remote_path` and `artifact_runs.remote_url`
- the step points to the produced `artifact_run`
- retry behavior follows the workflow engine contract for runner-owned steps

### 7.4 UI Verification

From Admin Web, confirm:

- `Generate from ...` buttons create real lineage-backed outputs
- `/settings/prompt-templates` can list and save templates
- `/ai-runs` shows status, tokens, cost, and lineage ids
- artifact viewer opens the generated artifact version
- approval-required steps wait for user approval instead of silently completing

### 7.5 Minimum Regression Matrix

Run these cases before closing the feature:

1. Happy-path generation with a valid source artifact run
2. Invalid source artifact run id
3. Unauthorized user or missing JWT
4. Approval-required generation step
5. Retry or regenerate from AI logs
6. Artifact save succeeds even if downstream memory extraction fails
