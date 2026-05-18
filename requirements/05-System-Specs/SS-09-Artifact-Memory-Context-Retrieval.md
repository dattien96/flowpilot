# FlowPilot - Artifact Memory & Context Retrieval

This document defines how FlowPilot turns saved workflow history into usable AI context.

## 1. Core Problem

FlowPilot already saves workflow runs, logs, approvals, and raw artifact files. This gives the user traceability, but it does not automatically make the next AI model execution aware of the right context.

The system must distinguish between:
- **Durable memory:** the complete raw artifact file stored as the source of truth.
- **Working memory:** a short, structured representation of the artifact that can be searched, filtered, and ranked.
- **Prompt memory:** the final selected context sent to the AI model for one workflow step.

The AI model should not receive every previous artifact by default. The model should receive the smallest useful context set for the current step.

## 2. Durable Memory

Durable memory is the original artifact output.

Rules:
- Every workflow step output is saved as a raw Markdown or JSON artifact file.
- The raw artifact file remains the single source of truth.
- Supabase Storage is the default storage backend.
- Google Drive can be used as an optional sync or backup target, but it does not replace the canonical artifact record unless explicitly configured.
- Raw artifact files are used for full viewing, audit, export, and re-indexing.

## 3. Working Memory

Working memory is a compact, structured summary generated from the raw artifact.

Purpose:
- Give the system a fast way to understand what an artifact contains.
- Support search, filtering, ranking, and prompt assembly.
- Avoid sending long raw artifacts to the model when a summary is enough.

Each artifact should produce one or more working memory records with fields such as:
- `ai_output_id` when using the current `ai_outputs` table as the artifact record
- `project_id`
- `workflow_id`
- `workflow_run_id`
- `step_id`
- `artifact_type`
- `status`
- `version`
- `summary`
- `key_decisions`
- `constraints`
- `assumptions`
- `open_questions`
- `keywords`
- `source_refs`
- `embedding`
- `token_estimate`

Working memory is stored in Supabase Postgres so the Admin Web can query it directly and the Go-Runner can retrieve it before prompt assembly.

## 4. Prompt Memory

Prompt memory is the runtime context sent to the AI provider.

Prompt memory is assembled per workflow step. It can include:
- mandatory step context
- workflow run summary
- latest approved previous-step artifact summary
- selected working memory records
- selected raw artifact excerpts when needed
- pinned human annotations
- rejection notes for retries
- MCP context such as Jira, Figma, Firebase, or Google Drive data

The prompt must include citations or source references for every selected artifact/memory item so the output can be traced back to its inputs.

## 5. Retrieval Rules

FlowPilot uses workflow-aware retrieval, not generic chat retrieval.

Before a step runs, the Context Resolver should:
- filter by project and workflow scope
- prefer the latest approved artifact versions
- include required previous-step outputs
- include pinned user notes and annotations
- retrieve semantically relevant working memory records with vector search
- expand to raw artifact excerpts only when summaries are insufficient
- respect a step-level token budget

Each step definition can configure:
- required artifact types
- optional artifact types
- whether full previous artifact content is required
- max working memory tokens
- max raw excerpt tokens
- whether only approved artifacts are allowed
- whether cross-workflow retrieval is allowed

## 6. RAG Usage

RAG is used for selecting relevant context. It is not the complete memory system.

FlowPilot's memory system consists of:
- raw artifact storage
- structured working memory generation
- embedding generation for working memory records
- vector search over working memory
- prompt packing with token budgets

Supabase vector search should be used to retrieve relevant working memory records. Raw artifact files are only loaded when the selected memory item needs expansion.

## 7. Artifact Management Screen

The Admin Web must include an Artifact Management screen.

Required capabilities:
- list artifacts by project, workflow, run, step, type, status, and version
- view raw artifact content
- view generated working memory
- view embedding/index status
- filter approved, rejected, draft, and superseded artifacts
- compare artifact versions
- show which artifact memories were used in a workflow run
- allow users to pin an artifact, annotation, or memory record as required context

## 8. Definition of Done

- Raw artifacts remain the single source of truth in Supabase Storage.
- Every completed artifact has a structured working memory record.
- Working memory records can be embedded and searched.
- Prompt assembly uses selected memory, not full workflow history by default.
- Users can inspect both raw artifacts and working memory in the Admin Web.
- Workflow runs record which memory items were injected into the model prompt.
