# CP-12: Artifact Memory, RAG & Prompt Context

**Maps from:** SS-09, SD-10, SD-08, SD-05
**Phase:** Cross-cutting, built after CP-07 and alongside CP-09
**Depends on:** CP-07, CP-06, CP-09

---

## 1. Core Concept

This phase closes the gap between "we saved the artifact" and "the next AI step knows the right context."

The implementation must support three memory layers:
- **Durable memory:** raw artifact files in Supabase Storage.
- **Working memory:** structured artifact summaries in Supabase Postgres.
- **Prompt memory:** selected context injected into the model prompt for one workflow step.

RAG is used to retrieve relevant working memory, but final prompt context is controlled by workflow rules and token budgets.

---

## 2. Database Migration

Create a new Supabase migration for:
- `artifact_memories`
- `workflow_prompt_context_items`
- `match_artifact_memories()` RPC
- `CREATE EXTENSION IF NOT EXISTS vector`
- vector index for artifact memory embeddings
- RLS policies matching project/team access rules

> **Important:** After the CP-06 redesign, all generated artifacts are stored as canonical `artifact_runs` linked to `artifact_definitions`, not `ai_outputs`. The `artifact_memories` FK must reference `artifact_runs(id)`.
> Use `vector(384)` when using Supabase `gte-small`. If another model is selected, align the vector dimension with that model.

Key DDL (refer to SD-10 §2 for full schema, update FKs as noted):
```sql
CREATE EXTENSION IF NOT EXISTS vector;

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

-- RLS
ALTER TABLE artifact_memories ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_prompt_context_items ENABLE ROW LEVEL SECURITY;

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
```

---

## 3. Supabase Edge Function

Create or reuse `generate-embedding`.

Use the known-working reference implementation documented in [CP-09 §3.5](./CP-09-AI-Orchestration.md).

Requirements:
- accepts `POST`
- validates a string `text` field
- uses `new Supabase.ai.Session("gte-small")`
- calls `session.run(text, { mean_pool: true, normalize: true })`
- returns `embedding` and `dimensions`
- handles CORS preflight
- returns structured error responses

The Edge Function should be used for:
- embedding working memory records
- embedding step queries before vector search
- JWT auth is required to prevent unauthenticated use.
- Project access control remains the responsibility of the caller before sending text to `generate-embedding`.

---

## 4. Working Memory Generation

After every successful artifact save:
1. Load the raw artifact content.
2. Generate structured memory.
3. Insert `artifact_memories` with `embedding_status = 'pending'`.
4. Call `generate-embedding`.
5. Store the embedding and mark `embedding_status = 'ready'`.

Structured memory fields:
- summary
- key decisions
- constraints
- assumptions
- open questions
- keywords
- source refs
- token estimate

Failure behavior:
- raw artifact save must still succeed if memory generation fails
- failed memory records should be visible in Admin Web
- failed embeddings can be retried

---

## 5. Context Resolver

Add a Go-Runner module:

```text
internal/contextresolver/
  policy.go
  resolver.go
  ranking.go
  packing.go
  audit.go
```

Responsibilities:
- load step retrieval policy
- include mandatory context
- search artifact memories by vector similarity
- rank results using workflow-aware boosts
- fit selected items into token budget
- request raw artifact excerpts only when needed
- return prompt sections to the Prompt Assembler
- write `workflow_prompt_context_items` audit rows

---

## 6. Step Retrieval Policy

Extend step definitions with context policy fields:

```ts
type StepContextPolicy = {
  requiredArtifactDefinitionKeys: string[];
  optionalArtifactDefinitionKeys: string[];
  requirePreviousArtifactFullContent: boolean;
  allowCrossWorkflowRetrieval: boolean;
  approvedOnly: boolean;
  maxWorkingMemoryTokens: number;
  maxRawExcerptTokens: number;
  maxRetrievedItems: number;
};
```

> **Implementation note:** The retrieval policy for each step is seeded alongside the `step_definitions` table (CP-07) — one policy record per `step_type`. At runtime the Go-Runner loads it via `getStepDefinition(step.step_type)`. For MVP, default values (see below) are used if no per-step override is configured.

Default policy:
- include previous approved artifact summary
- search same workflow only
- approved artifact runs only
- max 8 retrieved memory records
- prefer summaries over raw content

---

## 7. Prompt Assembly Update

Update the Prompt Assembler runtime sections:

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

The model prompt should include source references so generated output can be audited.

---

## 8. Admin Web

Add Artifact Management:
- artifact table with filters
- artifact run detail view
- working memory detail view
- embedding/indexing status
- version history
- compare versions
- pin/unpin artifact or memory as required context

Update Workflow Execution Dashboard:
- show prompt context drawer per step
- list memory items used in prompt
- show token estimates
- link memory records back to artifact runs and their raw storage locations

---

## 9. Testing

Unit tests:
- working memory extraction shape
- vector search RPC ranking inputs
- context ranking
- token budget packing
- audit row creation

Integration tests:
- raw artifact save creates memory
- memory embedding can be retried
- workflow step prompt includes required context
- prompt context audit shows selected memory

Manual validation:
- create a workflow with multiple artifacts
- approve an upstream artifact
- run a later step
- verify only relevant memory is injected
- verify raw artifact remains viewable as source of truth

---

## 10. Definition of Done

- [ ] `artifact_memories` table exists with `artifact_run_id UUID REFERENCES artifact_runs(id)` FK and vector search support.
- [ ] `workflow_prompt_context_items` table exists with `workflow_run_step_id UUID` FK.
- [ ] RLS SELECT policies on both tables (project-member scoped).
- [ ] `generate-embedding` Edge Function returns normalized embeddings.
- [ ] Working memory is generated after artifact save (non-fatal on failure).
- [ ] Failed memory records are visible in Admin Web with retry capability.
- [ ] Context Resolver selects memory by policy and token budget.
- [ ] Prompt Assembler receives packed prompt memory (not all raw artifacts).
- [ ] Each workflow step records the memory items used in `workflow_prompt_context_items`.
- [ ] Admin Web can inspect artifacts, working memory, embedding status, and prompt context usage.
