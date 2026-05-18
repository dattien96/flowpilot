# CP-09: Artifact Memory, RAG & Prompt Context

**Maps from:** SS-09, SD-10, SD-08, SD-05
**Phase:** Cross-cutting, built after CP-04 and alongside CP-06
**Depends on:** CP-04, CP-06

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

The current schema stores generated artifacts in `ai_outputs`; `artifact_memories` should reference `ai_outputs(id)`. Use `vector(384)` when using Supabase `gte-small`. If another model is selected, align the vector dimension with that model.

---

## 3. Supabase Edge Function

Create or reuse `generate-embedding`.

Use the known-working reference implementation documented in [CP-06 §3.5](./CP-06-AI-Orchestration.md).

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
  requiredArtifactTypes: string[];
  optionalArtifactTypes: string[];
  requirePreviousArtifactFullContent: boolean;
  allowCrossWorkflowRetrieval: boolean;
  approvedOnly: boolean;
  maxWorkingMemoryTokens: number;
  maxRawExcerptTokens: number;
  maxRetrievedItems: number;
};
```

Default policy:
- include previous approved artifact summary
- search same workflow only
- approved artifacts only
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
- raw artifact detail view
- working memory detail view
- embedding/indexing status
- version history
- compare versions
- pin/unpin artifact or memory as required context

Update Workflow Execution Dashboard:
- show prompt context drawer per step
- list memory items used in prompt
- show token estimates
- link memory records back to raw artifacts

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

- Raw artifacts continue to save to Supabase Storage.
- Working memory is generated and stored for each completed artifact.
- Working memory embeddings are searchable through Supabase vector search.
- Context Resolver builds prompt memory from selected records.
- Prompt assembly no longer relies on pushing all prior artifacts into the model.
- Admin Web exposes artifacts, working memory, embedding status, and prompt context usage.
