# FlowPilot Tech Design - Context Resolver & RAG

This document translates `SS-09-Artifact-Memory-Context-Retrieval.md` into technical implementation details.

## 1. Architecture Position

The Context Resolver sits between artifact storage and prompt assembly.

Flow:

```text
Raw Artifact in Supabase Storage
  -> Working Memory Extractor
  -> artifact_memories table
  -> Embedding Edge Function
  -> vector search
  -> Context Resolver
  -> Prompt Assembler
  -> AI Provider
```

The Go-Runner is responsible for calling the Context Resolver before each workflow step executes. Supabase stores the memory records, embeddings, and prompt-context audit logs.

## 2. Database Design

### 2.1 Artifact Memory Table

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE artifact_memories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ai_output_id TEXT NOT NULL REFERENCES ai_outputs(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  workflow_id UUID REFERENCES workflows(id) ON DELETE SET NULL,
  workflow_run_id UUID REFERENCES workflow_runs(id) ON DELETE SET NULL,
  workflow_run_step_id UUID REFERENCES workflow_run_steps(id) ON DELETE SET NULL,
  artifact_type TEXT NOT NULL,
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
CREATE INDEX artifact_memories_artifact_type_idx ON artifact_memories(artifact_type);
CREATE INDEX artifact_memories_embedding_idx
  ON artifact_memories
  USING ivfflat (embedding vector_cosine_ops)
  WITH (lists = 100);
```

The `vector(384)` size assumes the Supabase `gte-small` embedding model. If the embedding provider changes, the migration must use the matching dimension.

### 2.2 Prompt Context Audit Table

```sql
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
```

This table records what memory was sent to the model for every step. It supports auditability and debugging.

## 3. Working Memory Extraction

When a raw artifact is saved, the system should generate a structured working memory record.

Extractor responsibilities:
- read the raw artifact content
- produce a short summary
- extract key decisions, constraints, assumptions, open questions, and keywords
- compute a token estimate
- store source references back to the raw artifact
- mark `embedding_status = 'pending'`

The extractor can run in the Go-Runner or a Supabase Edge Function. For MVP, the Go-Runner can call the configured AI provider to produce the structured memory after artifact save, then call the embedding Edge Function.

## 4. Embedding Edge Function

Use a Supabase Edge Function named `generate-embedding`.

Input:

```json
{
  "text": "summary and structured memory text"
}
```

Output:

```json
{
  "embedding": [0.01, -0.02],
  "dimensions": 384
}
```

Implementation note:
- Use `new Supabase.ai.Session("gte-small")`.
- Use `mean_pool: true`.
- Use `normalize: true`.
- Validate request method and input.
- Return dimensions so migrations and debugging can detect mismatches.

The text sent for embedding should combine the most searchable fields:

```text
Artifact Type: <artifact_type>
Status: <status>
Summary: <summary>
Key Decisions: <key_decisions>
Constraints: <constraints>
Assumptions: <assumptions>
Open Questions: <open_questions>
Keywords: <keywords>
```

## 5. Context Resolver

The Context Resolver builds the prompt memory for a step.

Inputs:
- project ID
- workflow ID
- workflow run ID
- current step definition
- current task text
- user context
- MCP context
- retry/rejection note
- step retrieval policy
- token budget

Output:
- selected context items
- packed prompt sections
- audit records for `workflow_prompt_context_items`

Selection order:
- required step context
- latest approved immediate previous-step artifact
- pinned user annotations or pinned artifact memories
- workflow run summary memory
- top vector search results from `artifact_memories`
- raw excerpts only for selected items that need full detail

## 6. Vector Search

The resolver should generate an embedding for the current step query and search `artifact_memories`.

Example RPC shape:

```sql
CREATE OR REPLACE FUNCTION match_artifact_memories(
  query_embedding vector(384),
  match_project_id UUID,
  match_workflow_id UUID,
  match_count INT DEFAULT 10
)
RETURNS TABLE (
  id UUID,
  ai_output_id TEXT,
  summary TEXT,
  artifact_type TEXT,
  similarity DOUBLE PRECISION
)
LANGUAGE SQL STABLE
AS $$
  SELECT
    am.id,
    am.ai_output_id,
    am.summary,
    am.artifact_type,
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

Ranking should combine:
- semantic similarity
- approved/latest status
- required artifact type match
- recency
- pinned context boost
- same workflow/run boost

## 7. Prompt Packing

Prompt packing converts selected memory into final prompt sections.

Priority:
- Tier 1: mandatory context, current task, current step rules
- Tier 2: latest approved previous artifact summary
- Tier 3: workflow memory and selected working memory records
- Tier 4: raw artifact excerpts
- Tier 5: optional historical context

If token budget is exceeded:
- keep Tier 1
- keep pinned context
- compress summaries
- drop lower-ranked optional records
- avoid loading raw artifacts unless required

## 8. Admin Web UI

Add an Artifact Management screen with these views:
- artifact list
- raw artifact viewer
- working memory viewer
- memory indexing status
- context usage history
- filters by project, workflow, run, step, type, status, version, and embedding status

Add a prompt context drawer in the Workflow Execution Dashboard:
- shows which memory records were sent to the model
- shows token estimates
- links back to raw artifacts
- shows why each item was selected when possible

## 9. Security

RLS must enforce project ownership or project team access for:
- artifacts
- artifact memories
- prompt context audit records

Embedding Edge Functions must not allow a user to embed or retrieve data from another user's project.

## 10. Definition of Done

- `artifact_memories` table exists with vector search support.
- `generate-embedding` Edge Function returns normalized embeddings.
- Working memory is generated after artifact save.
- Context Resolver selects memory by policy and budget.
- Prompt Assembler receives packed prompt memory.
- Each workflow step records the memory items used.
- Admin Web can inspect artifacts, working memory, and prompt context usage.
