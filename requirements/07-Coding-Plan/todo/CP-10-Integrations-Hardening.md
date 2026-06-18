# CP-10: Integrations, Memory & Context Intelligence

**Maps from:** SD-03, SD-04 §4, SS-03, CP-05, SS-09, SD-10, SD-08, SD-05
**Phase:** 7 (final hardening + cross-cutting context intelligence)
**Depends on:** CP-09, CP-07, CP-06
**Additional input:** SS-13 (AI-followable document contract for governed phase docs)

input = artifact + mcp + history + Rag
output = artifact + rag

RAG nhu nao ? Nen luu 1 he thong file nhu kieu danh ba
feature > layer (data-domain-ui)
Tat ca tongr hop lai thanh 1 bo id -> convert thanh 1 VECTOR
push len RAG supabase
key: vector -> id -> id cua file
get duoc file thi se biet ben trong summary change for feat nhu nao
De lam dc cai nay thi can co flow va doc chuan format

---

## 1. Core Concept

This phase covers two closely related concerns that must ship together:

**Part A — Integration Hardening:** Harden the external systems (Jira, Firebase, Google Drive, Telegram) configured in CP-05 with stricter RLS, real sync logic, and audit trails.

**Part B — Artifact Memory & Context Intelligence:** Close the gap between "we saved the artifact" and "the next AI step knows the right context." Build the memory pipeline (working memory, embeddings, context slots, HyperRAG) that makes FlowPilot's context accumulation real.

These two parts are merged here because:

- Google Drive sync (Part A) feeds the `drive.file` context slot resolver (Part B).
- Both share the same audit log table and RLS hardening pass.
- Both are best shipped together as one coherent phase.

CP-10 must not move MCP configuration ownership out of CP-05.
CP-10 must treat governed project documents that follow `SS-13` as structured context, not generic markdown blobs.

---

## 2. Part A — Integration Hardening

### 2.1 Jira

#### Database

```sql
-- Cache Jira members for project syncing
CREATE TABLE jira_members_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  jira_account_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  email TEXT,
  avatar_url TEXT,
  synced_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(project_id, jira_account_id)
);

-- Cache Jira issues for status syncing
CREATE TABLE jira_issues_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  jira_issue_key TEXT NOT NULL,
  jira_status TEXT,
  jira_summary TEXT,
  synced_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(project_id, jira_issue_key)
);

-- NOTE: The `integrations` table already exists from CP-05.
-- Do NOT re-create or move it here. CP-10 only hardens policies and adds sync/cache tables.

ALTER TABLE jira_members_cache ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_issues_cache ENABLE ROW LEVEL SECURITY;

CREATE POLICY "jira_members_cache_select"
  ON jira_members_cache FOR SELECT
  USING (
    project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  );

CREATE POLICY "jira_issues_cache_select"
  ON jira_issues_cache FOR SELECT
  USING (
    project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  );

CREATE POLICY "integrations_select_project_members"
  ON integrations FOR SELECT
  USING (
    project_id IN (
      SELECT p.id FROM projects p
      WHERE p.created_by = auth.uid() OR p.owner_id = auth.uid()
    )
  );
```

#### Edge Functions

```
POST /functions/v1/jira-import-members
Body: { projectId, integrationId }
→ Calls Jira REST API → upserts into jira_members_cache
→ Optionally maps to team_members by jira_account_id

POST /functions/v1/jira-create-issue
Body: { projectId, taskId, integrationId }
→ Creates Jira issue from task → stores jira_issue_key on task

POST /functions/v1/jira-sync-status
Body: { projectId, integrationId }
→ Reads Jira issue statuses → updates jira_issues_cache
→ Optionally syncs back to tasks.status
```

#### UI

- `/projects/:projectId/members` → "Import from Jira" button (now enabled)
- `/projects/:projectId/tasks` → "Create Jira Issue" on each task card
- `/projects/:projectId/tasks` → Jira status badge sync indicator
- `/projects/:projectId/settings` → Jira integration configuration

---

### 2.2 Firebase

- **Edge Function:** `firebase-fetch-crashes` — pulls crash logs for Issue Analysis step
- **Edge Function:** `firebase-fetch-analytics` — pulls usage data for Analytics Review step
- **Config:** Firebase project ID + service account key (stored encrypted in `integrations`)

---

### 2.3 Google Drive

- **Edge Function:** `google-drive-sync-artifacts` — uploads artifact runs to a configured Drive folder after each step completes. Uses path: `{remote_sync_root_path}/{projectId}/{workflowRunId}/{stepType}/{file_name}`.
- **Edge Function:** `google-drive-sync-skills` — syncs `.claude/`, `.codex/`, `.gemini/` skill folders to Drive.
- **Context slot link:** Once a file is synced, its Drive file ID is stored on `artifact_runs.remote_url`. The `drive.file` context slot resolver uses this ID to fetch file content at runtime — no re-upload needed.
- **Config:** OAuth2 tokens stored securely on Go-Runner, not in Supabase.

---

### 2.4 Telegram

- **Edge Function:** `telegram-send-notification` — sends workflow status updates to a configured Telegram chat
- **Config:** Bot token + chat ID (stored encrypted)
- **Trigger:** Automatically called when a workflow step enters `WAITING_USER_APPROVAL` or `DONE`

---

## 3. Part B — Artifact Memory & Context Intelligence

### 3.1 Database Migration

Create one Supabase migration containing:

```sql
CREATE EXTENSION IF NOT EXISTS vector;

-- Working memory: structured summary + embedding per artifact run
CREATE TABLE artifact_memories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  artifact_run_id UUID NOT NULL REFERENCES artifact_runs(id) ON DELETE CASCADE,
  artifact_definition_id UUID REFERENCES artifact_definitions(id) ON DELETE SET NULL,
  artifact_definition_key TEXT NOT NULL,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  workflow_id UUID REFERENCES workflows(id) ON DELETE SET NULL,
  workflow_run_id UUID REFERENCES workflow_runs(id) ON DELETE SET NULL,
  workflow_run_step_id UUID REFERENCES workflow_run_steps(id) ON DELETE SET NULL,
  artifact_status TEXT NOT NULL,
  artifact_version INT NOT NULL,
  summary TEXT NOT NULL,
  key_decisions JSONB NOT NULL DEFAULT '[]',
  constraints JSONB NOT NULL DEFAULT '[]',
  assumptions JSONB NOT NULL DEFAULT '[]',
  open_questions JSONB NOT NULL DEFAULT '[]',
  keywords JSONB NOT NULL DEFAULT '[]',
  source_refs JSONB NOT NULL DEFAULT '[]',
  token_estimate INT NOT NULL DEFAULT 0,
  embedding vector(384),
  embedding_model TEXT,
  embedding_status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX artifact_memories_project_idx ON artifact_memories(project_id);
CREATE INDEX artifact_memories_workflow_idx ON artifact_memories(workflow_id);
CREATE INDEX artifact_memories_artifact_definition_key_idx ON artifact_memories(artifact_definition_key);
CREATE INDEX artifact_memories_embedding_idx
  ON artifact_memories
  USING ivfflat (embedding vector_cosine_ops)
  WITH (lists = 100);

-- Audit: which memory items were injected into each step's prompt
CREATE TABLE workflow_prompt_context_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_run_step_id UUID NOT NULL REFERENCES workflow_run_steps(id) ON DELETE CASCADE,
  source_type TEXT NOT NULL,
  source_id TEXT,
  source_ref TEXT,
  included_as TEXT NOT NULL,
  rank_score DOUBLE PRECISION,
  token_estimate INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Context slot definitions per step: one row per slot, managed via Admin UI dropdowns
CREATE TABLE step_context_slots (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  step_definition_id  UUID NOT NULL REFERENCES workflow_step_definitions(id) ON DELETE CASCADE,
  name                TEXT NOT NULL,
  resolver            TEXT NOT NULL,   -- see resolver types in §3.4
  priority            INT  NOT NULL DEFAULT 2,  -- 1=always | 2=if budget | 3=trim first
  max_tokens          INT  NOT NULL DEFAULT 1000,
  required            BOOLEAN NOT NULL DEFAULT FALSE,
  order_index         INT  NOT NULL DEFAULT 0,
  resolver_config     JSONB NOT NULL DEFAULT '{}',  -- set by UI forms, never hand-edited
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(step_definition_id, name)
);

CREATE INDEX step_context_slots_step_def_idx ON step_context_slots(step_definition_id);

-- RLS
ALTER TABLE artifact_memories ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_prompt_context_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE step_context_slots ENABLE ROW LEVEL SECURITY;

CREATE POLICY "artifact_memories_select_project_members"
  ON artifact_memories FOR SELECT
  USING (
    project_id IN (
      SELECT pt.project_id FROM project_teams pt
      JOIN team_members tm ON tm.team_id = pt.team_id
      WHERE tm.user_id = auth.uid()
    )
  );

CREATE POLICY "prompt_context_items_select_project_members"
  ON workflow_prompt_context_items FOR SELECT
  USING (
    workflow_run_step_id IN (
      SELECT wrs.id FROM workflow_run_steps wrs
      JOIN workflow_runs wr ON wr.id = wrs.workflow_run_id
      WHERE wr.project_id IN (
        SELECT pt.project_id FROM project_teams pt
        JOIN team_members tm ON tm.team_id = pt.team_id
        WHERE tm.user_id = auth.uid()
      )
    )
  );

-- step_context_slots: global step definitions are readable by all authenticated users
CREATE POLICY "step_context_slots_select_authenticated"
  ON step_context_slots FOR SELECT
  TO authenticated
  USING (true);

-- Vector search RPC
CREATE OR REPLACE FUNCTION match_artifact_memories(
  query_embedding vector(384),
  match_project_id UUID,
  match_workflow_id UUID,
  match_count INT DEFAULT 10
)
RETURNS TABLE (
  id UUID,
  artifact_run_id UUID,
  summary TEXT,
  artifact_definition_key TEXT,
  similarity DOUBLE PRECISION
)
LANGUAGE SQL STABLE
AS $$
  SELECT
    am.id,
    am.artifact_run_id,
    am.summary,
    am.artifact_definition_key,
    1 - (am.embedding <=> query_embedding) AS similarity
  FROM artifact_memories am
  WHERE am.project_id = match_project_id
    AND (match_workflow_id IS NULL OR am.workflow_id = match_workflow_id)
    AND am.embedding IS NOT NULL
    AND am.embedding_status = 'ready'
  ORDER BY am.embedding <=> query_embedding
  LIMIT match_count;
$$;
```

> **Important:** `artifact_memories` FK references `artifact_runs(id)`, not `ai_outputs`. Use `vector(384)` for Supabase `gte-small`. Adjust dimension if switching embedding model.

---

### 3.2 generate-embedding Edge Function

Create or reuse `generate-embedding`. Reference the known-working implementation in [CP-09 §3.5](./DONE-CP-09-AI-Orchestration.md).

Requirements:

- accepts `POST`
- validates a string `text` field
- uses `new Supabase.ai.Session("gte-small")`
- calls `session.run(text, { mean_pool: true, normalize: true })`
- returns `{ embedding, dimensions }`
- handles CORS preflight
- JWT auth required

Used for:

- embedding working memory records after artifact save
- embedding the HyperRAG search query before vector search

---

### 3.3 Working Memory Generation

After every successful artifact save, the Go-Runner must:

1. Load raw artifact content from local file.
2. Call the AI provider to extract structured memory fields:
   - `summary`, `key_decisions`, `constraints`, `assumptions`, `open_questions`, `keywords`, `source_refs`, `token_estimate`
3. Insert into `artifact_memories` with `embedding_status = 'pending'`.
4. Call `generate-embedding` with the combined memory text.
5. Store the embedding and set `embedding_status = 'ready'`.

Structured phase-document fast path:

- For markdown files in `requirements/05-System-Specs`, `06-System-Tech-Design`, `07-Coding-Plan`, `08-Task`, and `09-BugFix`, first parse the `SS-13` contract deterministically before falling back to full AI extraction.
- Read the metadata block and `AI Quick View` block when present.
- Map `AI Quick View` fields directly:
  - `Summary` -> `summary`
  - `Key Decisions` -> `key_decisions`
  - `Constraints` -> `constraints`
  - `Open Questions` -> `open_questions`
  - `Source Refs` -> `source_refs`
- Add metadata fields such as `Document ID`, `Phase`, `Parent Documents`, and `Tags` into derived `keywords` and `source_refs` so retrieval can trace document lineage cheaply.
- Use model-generated extraction only to fill gaps, normalize legacy non-compliant docs, or derive fields that are not explicitly present, such as `assumptions`.

Failure behavior:

- Raw artifact save must succeed even if memory generation fails.
- Failed memory records must be visible in Admin Web with retry capability.

---

### 3.4 Context Slot Model

Each workflow step declares its context needs as rows in `step_context_slots` — one row per slot.

The Admin UI manages these rows through a form with dropdowns. The `resolver_config` JSONB is set by UI form fields, never hand-edited.

#### Supported resolver types

| Resolver              | Fetch Source                                       | Behavior                                      |
| --------------------- | -------------------------------------------------- | --------------------------------------------- |
| `project.brief`       | `projects.brief` column                            | Always available. Short project summary.      |
| `run.input`           | Workflow run intake form                           | The user's original task/feature description. |
| `step.previous.brief` | Previous step's `artifact_memories.summary`        | Automatic continuity.                         |
| `step.n.brief`        | Step N's `artifact_memories.summary`               | Non-adjacent step reference by index.         |
| `artifact.required`   | `artifact_runs` lookup by definition key           | Deterministic. Fails step if missing.         |
| `semantic.search`     | pgvector search over `artifact_memories`           | Dynamic. Uses HyperRAG query.                 |
| `drive.file`          | Drive file ID stored on `artifact_runs.remote_url` | Specific synced document.                     |
| `mcp.context`         | Live MCP call (Jira, Figma, Firebase, etc.)        | Real-time external data.                      |
| `static`              | Hardcoded string in `resolver_config.content`      | Shared prompt rules or fragments.             |

#### resolver_config per resolver type

| Resolver                                              | resolver_config                                                  | UI controls                                   |
| ----------------------------------------------------- | ---------------------------------------------------------------- | --------------------------------------------- |
| `project.brief` / `run.input` / `step.previous.brief` | `{}`                                                             | None                                          |
| `step.n.brief`                                        | `{ "step_index": 2 }`                                            | Step index number input                       |
| `artifact.required`                                   | `{ "artifact_key": "tech_spec_artifact" }`                       | Artifact definition dropdown                  |
| `semantic.search`                                     | `{ "query_template": "...", "top_k": 3, "use_hyperrag": false }` | Template input, top_k slider, HyperRAG toggle |
| `drive.file`                                          | `{ "file_path": "docs/arch.md" }`                                | File path input                               |
| `mcp.context`                                         | `{ "mcp_provider": "jira" }`                                     | MCP provider dropdown                         |
| `static`                                              | `{ "content": "Always respond in English." }`                    | Multi-line text area                          |

#### Default slots seeded for all MVP steps

| name                  | resolver              | priority | max_tokens | resolver_config                                                                 |
| --------------------- | --------------------- | -------- | ---------- | ------------------------------------------------------------------------------- |
| `project_brief`       | `project.brief`       | 1        | 500        | `{}`                                                                            |
| `run_input`           | `run.input`           | 1        | 1000       | `{}`                                                                            |
| `previous_step_brief` | `step.previous.brief` | 2        | 1500       | `{}`                                                                            |
| `relevant_past`       | `semantic.search`     | 3        | 3000       | `{ "query_template": "{{step.description}} {{run.feature_name}}", "top_k": 3 }` |

Steps with MCP requirements add one extra row:

| name          | resolver      | priority | max_tokens | required | resolver_config              |
| ------------- | ------------- | -------- | ---------- | -------- | ---------------------------- |
| `jira_ticket` | `mcp.context` | 2        | 2000       | true     | `{ "mcp_provider": "jira" }` |

---

### 3.5 Context Resolver (Go-Runner)

Add a Go-Runner module:

```text
internal/contextresolver/
  policy.go      // loads step_context_slots rows and input artifact definitions
  slots.go       // dispatches each slot to the correct resolver function
  resolvers.go   // one resolver per slot type
  hyperrag.go    // HyperRAG query construction (template substitution + optional micro-call)
  ranking.go     // multi-factor ranking (similarity, recency, pinned, same-run boost)
  packing.go     // token budget packing in priority order
  audit.go       // writes workflow_prompt_context_items rows
```

Processing order per step execution:

1. Load all `step_input_artifact_definitions` — resolve each as `artifact.required` (fail-fast if missing).
2. Load `step_context_slots` ordered by `priority ASC, order_index ASC`.
3. Resolve Priority 1 slots (project brief, run input).
4. Resolve Priority 2 slots (previous step brief, MCP context).
5. Resolve Priority 3 slots (semantic search).
6. Pack resolved items into prompt within token budget.
7. Write one `workflow_prompt_context_items` row per resolved slot.

---

### 3.6 HyperRAG Query Construction

For `semantic.search` slots, the resolver constructs a short focused query (10–50 tokens) before calling pgvector. Embedding a long prompt produces a blurry, averaged vector.

**Step 1 — Template substitution (default):**

```text
resolver_config.query_template: "{{step.description}} {{run.feature_name}}"
Resolved: "Design system architecture for Real-time notifications"
→ embed → search artifact_memories
```

Supported variables:

| Variable               | Source                             |
| ---------------------- | ---------------------------------- |
| `{{step.description}}` | Step definition description        |
| `{{run.feature_name}}` | Workflow run intake field          |
| `{{run.goal}}`         | Workflow run intake field          |
| `{{project.name}}`     | Project name                       |
| `{{artifact.type}}`    | Step's primary output artifact key |

**Step 2 — AI-reformulated query (optional, `use_hyperrag: true`):**

```text
Micro-prompt (~100 tokens):
"In 10 words or less, what past decisions or context would help with:
[step description] for [run.feature_name]?"

Response: "notification backend websocket architecture previous decisions"
→ use this as the search query
```

Controlled by `resolver_config.use_hyperrag`. Off by default for MVP.

---

### 3.7 Prompt Assembly

The Prompt Assembler constructs the final prompt sections from resolved context:

```markdown
# Workflow Context

[step position, workflow name, run status]

# Selected Working Memory

[structured memories selected by Context Resolver]

# Source Artifacts

[links/references to raw artifact files]

# Raw Artifact Excerpts

[only when required by policy or token budget allows]
```

Each section includes source references so the output can be audited back to its inputs.

For governed phase documents that follow `SS-13`, prompt packing should prefer:

1. metadata block
2. `AI Quick View`
3. exact cited section IDs or subsections
4. raw excerpts only when the compact structure is insufficient

This keeps spec, design, plan, task, and bug documents cheap to retrieve and easier for the AI to follow consistently.

---

## 4. Security Hardening

### 4.1 RLS Policy Audit

All existing permissive `USING (true)` policies must be tightened. New tables from this phase (artifact_memories, workflow_prompt_context_items, step_context_slots, jira caches) must have RLS from creation.

```sql
-- Projects: visible only to owner
CREATE POLICY "projects_select_own" ON projects
  FOR SELECT TO authenticated
  USING (created_by = auth.uid() OR owner_id = auth.uid());

-- Features: restricted to project owner/admin
CREATE POLICY "features_select_project_member" ON features
  FOR SELECT TO authenticated
  USING (
    project_id IN (
      SELECT id FROM projects
      WHERE created_by = auth.uid() OR owner_id = auth.uid()
    )
  );
```

### 4.2 Input Validation (Zod)

All forms must validate with Zod before submitting:

```typescript
// src/lib/validators/project.ts
export const createProjectSchema = z.object({
  name: z.string().min(3).max(100),
  description: z.string().min(10).max(500),
  platform: z.enum(["android", "ios", "web", "multi"]),
  repositoryUrl: z.string().url(),
  directoryPath: z.string().optional(),
});

// src/lib/validators/context-slot.ts
export const contextSlotSchema = z.object({
  name: z.string().min(1).max(100),
  resolver: z.enum([
    "project.brief",
    "run.input",
    "step.previous.brief",
    "step.n.brief",
    "artifact.required",
    "semantic.search",
    "drive.file",
    "mcp.context",
    "static",
  ]),
  priority: z.union([z.literal(1), z.literal(2), z.literal(3)]),
  max_tokens: z.number().min(100).max(8000),
  required: z.boolean().default(false),
  resolver_config: z.record(z.unknown()).default({}),
});
```

### 4.3 Security Checklist

- [ ] No AI API keys in frontend code (all in Edge Function env vars)
- [ ] No Supabase service role key in frontend
- [ ] All forms validated with Zod before mutation
- [ ] RLS policies restrict data to project members
- [ ] Edge Functions validate JWT auth header
- [ ] Prompt template editing restricted to admin/owner role
- [ ] Integration config (tokens) encrypted at rest
- [ ] `resolver_config` content validated server-side before use in prompts

---

## 5. Audit Trail & Observability

### 5.1 Audit Log Table

```sql
CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id),
  action TEXT NOT NULL,         -- create_project, approve_step, reject_step, generate_spec, etc.
  resource_type TEXT NOT NULL,  -- project, workflow_run, artifact_run, etc.
  resource_id UUID NOT NULL,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "audit_logs_select_own"
  ON audit_logs FOR SELECT
  USING (user_id = auth.uid());

CREATE POLICY "audit_logs_insert_authenticated"
  ON audit_logs FOR INSERT
  WITH CHECK (auth.role() = 'authenticated');
```

### 5.2 Auto-logging Events

Write audit entries for:

- Project create/update/delete
- Workflow run start/complete/fail
- Approval decisions (approve/reject with comments)
- AI generation runs (model, cost, step)
- Task assignment changes
- Integration connect/disconnect
- Context slot resolution failures (required slot returned no result)
- Artifact memory generation failures (non-fatal)
- Embedding generation failures and retries

---

## 6. Error Handling & UX Polish

### 6.1 Global Error Boundary

```typescript
// src/components/common/error-boundary.tsx
export function ErrorBoundary({ error }: { error: Error }) {
  return (
    <Card className="p-8 text-center">
      <h2>Something went wrong</h2>
      <p>{error.message}</p>
      <Button onClick={() => window.location.reload()}>Retry</Button>
    </Card>
  )
}
```

### 6.2 Loading & Empty States

- Skeleton loaders for all DataTables and detail views
- Empty state illustrations with call-to-action buttons
- Toast notifications for mutations (success/error)

### 6.3 Responsive Layout

- Sidebar collapses on smaller screens
- DataTables switch to card layout on mobile if needed

---

## 7. Admin Web

### 7.1 Artifact Management screen

- Artifact table with filters (project, workflow, run, step, type, status, version, embedding status)
- Artifact run detail view with raw content viewer
- Working memory detail view (summary, key decisions, constraints, assumptions)
- Embedding/indexing status with retry button for failed embeddings
- Version history and version comparison
- Pin/unpin artifact or memory as required context

### 7.2 Step Context Slots panel (in step create/edit)

- Lists existing slots as rows: name | resolver badge | priority badge | max_tokens | required indicator
- **Add Slot** button opens a form:
  - Name (text input)
  - Resolver (dropdown — all 9 resolver types)
  - Priority (dropdown: `1 — Always` / `2 — If budget` / `3 — Trim first`)
  - Max tokens (number input)
  - Required (checkbox)
  - Resolver-specific fields that appear **conditionally** based on selected resolver
- Rows are reorderable (drag or up/down arrows set `order_index`)
- Delete slot removes the row
- No raw JSON editing exposed to users

### 7.3 Prompt Context drawer (in Workflow Execution Dashboard)

- Shows which memory records were sent to the model for each step
- Shows token estimates per slot
- Links memory records back to artifact runs and raw storage
- Explains why each item was selected (slot name, resolver, priority)

---

## 8. Testing

Unit tests:

- Working memory extraction produces expected shape
- Governed phase-document parser extracts metadata block and `AI Quick View` fields deterministically
- Vector search RPC ranking inputs are correct
- Context ranking combines similarity + recency + pinned boost correctly
- Token budget packing respects priority order
- Audit row creation per resolved slot

Integration tests:

- Raw artifact save → working memory generated → embedding stored
- Failed memory generation: artifact save still succeeds, retry visible in Admin Web
- Compliant `SS`/`SD`/`CP`/`Task`/`BugFix` docs use the structured extraction fast path before whole-file fallback
- Workflow step prompt includes required context slots
- Prompt context audit shows correct selected memory for each step

Manual validation:

- Create a workflow with multiple artifacts
- Approve an upstream artifact
- Run a later step
- Verify only relevant memory is injected (check prompt context drawer)
- Verify a governed phase document is represented by document ID, phase, parent links, and `AI Quick View` content instead of a whole-file dump when the compact structure is sufficient
- Verify raw artifact remains viewable as source of truth

---

## 9. Definition of Done

### Part A — Integration Hardening

- [ ] Jira: import members, create issues, sync status via Edge Functions
- [ ] Firebase: fetch crashes and analytics via Edge Functions
- [ ] Google Drive: artifact sync uploads artifact runs; Drive file ID stored on `artifact_runs.remote_url`
- [ ] Telegram: workflow notifications sent on step state change

### Part B — Artifact Memory & Context Intelligence

- [ ] `artifact_memories` table with `artifact_run_id` FK and vector search support
- [ ] `workflow_prompt_context_items` table with `workflow_run_step_id` FK
- [ ] `step_context_slots` table with FK to `workflow_step_definitions`, RLS enabled
- [ ] All MVP step definitions seeded with default context slot rows
- [ ] Steps with MCP requirements seeded with their MCP slot row
- [ ] `generate-embedding` Edge Function returns normalized embeddings
- [ ] Working memory generated after artifact save (non-fatal on failure)
- [ ] Governed phase documents in `requirements/05` to `09` use deterministic `SS-13` parsing before whole-file AI extraction fallback
- [ ] Failed memory records visible in Admin Web with retry capability
- [ ] Context Resolver loads slots from `step_context_slots` ordered by priority and order_index
- [ ] Context Resolver dispatches each slot type to the correct resolver
- [ ] HyperRAG template substitution resolves `resolver_config.query_template` before embedding
- [ ] Semantic search uses resolved short query, not the full prompt
- [ ] Context Resolver packs selected memory within token budget by priority
- [ ] Prompt Assembler receives packed prompt memory, not all raw artifacts
- [ ] Prompt packing prefers metadata + `AI Quick View` + exact cited sections for governed phase documents when that structure is available
- [ ] Each workflow step records memory items used in `workflow_prompt_context_items`
- [ ] Admin Web: Artifact Management screen with filters, memory viewer, embedding status
- [ ] Admin Web: Context Slots panel with Add/Edit/Delete/Reorder — no raw JSON exposed
- [ ] Admin Web: Prompt Context drawer shows what was injected and why

### Shared

- [ ] RLS policies hardened — no more `using (true)` on any table
- [ ] All forms validated with Zod (including context slot form)
- [ ] Audit log table with auto-logging for all key events
- [ ] Context slot resolution failures and embedding retries logged to audit
- [ ] Error boundaries, loading states, empty states
- [ ] No secrets in frontend code
- [ ] Edge Functions validate JWT auth headers
