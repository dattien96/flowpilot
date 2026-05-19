# Task-01: Execution Order

This document defines the recommended execution order for the FlowPilot refactor and feature buildout.

## 1. Recommended Order

```text
R1 -> CP-01 -> CP-02 -> CP-03 -> CP-04 -> CP-05 -> CP-07 -> CP-11A basic runner -> CP-06 -> CP-08 -> CP-09 -> CP-12 -> CP-11B full runner -> CP-10
```

## 2. Step Details

| Order | Plan | Purpose |
|---|---|---|
| 1 | [R1 Refactor To React](../10-Refactor/R1-Refactor-To-React-Plan.md) | Migrate `apps/admin-web` from Next.js to Vite + TanStack. |
| 2 | [CP-01 Foundation Setup](../07-Coding-Plan/CP-01-Foundation-Setup.md) | Validate the new frontend foundation: Vite, routing, auth, layout, and query setup. |
| 3 | [CP-02 Admin-Web Hot Reload](../07-Coding-Plan/CP-02-Admin-Web-Hot-Reload.md) | Make Docker dev loops stable so file changes reload reliably. |
| 4 | [CP-03 Supabase Integration](../07-Coding-Plan/CP-03-Supabase-Integration.md) | Lock down browser/server Supabase boundaries, storage, and Edge Functions. |
| 5 | [CP-04 Project Management](../07-Coding-Plan/CP-04-Project-Management.md) | Build the UUID-aligned project/team/member baseline and project settings shell. |
| 6 | [CP-05 Project MCP Context](../07-Coding-Plan/CP-05-Project-Mcp-Context.md) | Create the `integrations` table and build project-level MCP configuration/runtime loading. |
| 7 | [CP-07 Workflow Engine UI](../07-Coding-Plan/CP-07-Workflow-Engine-UI.md) | Create canonical workflow/run/step state, then build workflow builder, approvals, retries, and YOLO flow controls. |
| 8 | [CP-11 Go-Runner Implementation](../07-Coding-Plan/CP-11-Go-Runner-Implementation.md) | Implement the basic runner once workflow run/step state exists. Full context memory integration is deferred until CP-12. |
| 9 | [CP-06 Document Workflow](../07-Coding-Plan/CP-06-Document-Workflow.md) | Build the business logic, tech spec, and coding plan document artifact pipeline. |
| 10 | [CP-08 Master Schedule Tasks](../07-Coding-Plan/CP-08-Master-Schedule-Tasks.md) | Build master schedule and task board after workflow/document basics are stable. |
| 11 | [CP-09 AI Orchestration](../07-Coding-Plan/CP-09-AI-Orchestration.md) | Add AI prompt templates, execution logs, Edge Functions, and schedule/document generation. |
| 12 | [CP-12 Artifact Memory RAG](../07-Coding-Plan/CP-12-Artifact-Memory-RAG.md) | Add embeddings, working memory, vector search, and prompt context audit. |
| 13 | [CP-11 Go-Runner Implementation](../07-Coding-Plan/CP-11-Go-Runner-Implementation.md) | Expand runner with artifact memory/context resolver integration. |
| 14 | [CP-10 Integrations Hardening](../07-Coding-Plan/CP-10-Integrations-Hardening.md) | Harden integrations, audit, RLS, validation, and production polish. |

## 3. Guardrails

- CP-04 owns the UUID-aligned base schema. Do not add new FK-heavy tables against the legacy `TEXT` IDs.
- CP-05 owns all MCP/integration config, including the `integrations` table. CP-10 must not create or move MCP config ownership.
- Do not start CP-06 document artifacts before CP-07 has canonical workflow run/step state.
- Do not start CP-12 before CP-09 has `generate-embedding` and artifact records ready.
- CP-11 can start in basic mode after CP-07 schema exists, then expand after CP-12 adds prompt memory.
- CP-10 is last, but basic RLS and authorization checks should be reviewed during every database migration.

## 4. Practical Milestones

| Milestone | Plans | Outcome |
|---|---|---|
| Frontend foundation | R1, CP-01 | Admin Web runs on Vite + TanStack. |
| Platform foundations | CP-02, CP-03, CP-04, CP-05 | Docker reload, Supabase boundaries, project/team management, and MCP context wiring are ready. |
| Workflow foundation | CP-07 | Canonical workflow definitions, run steps, approvals, and prompt cache state exist. |
| Executable workflows | CP-11A basic runner | Workflow runs can be picked up and executed by the Go-Runner without artifact memory. |
| Core product shell | CP-06, CP-08 | Users can manage documents, schedules, and tasks. |
| AI memory layer | CP-09, CP-12, CP-11B | Artifacts become searchable working memory and selected prompt context. |
| Production readiness | CP-10 | Integrations, security, audit, and hardening are complete. |

