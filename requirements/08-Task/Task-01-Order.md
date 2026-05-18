# Task-01: Execution Order

This document defines the recommended execution order for the FlowPilot refactor and feature buildout.

## 1. Recommended Order

```text
R1 -> CP-01 -> CP-02 -> CP-03 -> CP-04 -> CP-05 -> CP-06 -> CP-07 -> CP-11 basic runner -> CP-09 + CP-12 -> CP-08 -> CP-10
```

## 2. Step Details

| Order | Plan | Purpose |
|---|---|---|
| 1 | [R1 Refactor To React](../10-Refactor/R1-Refactor-To-React-Plan.md) | Migrate `apps/admin-web` from Next.js to Vite + TanStack. |
| 2 | [CP-01 Foundation Setup](../07-Coding-Plan/CP-01-Foundation-Setup.md) | Validate the new frontend foundation: Vite, routing, auth, layout, and query setup. |
| 3 | [CP-02 Admin-Web Hot Reload](../07-Coding-Plan/CP-02-Admin-Web-Hot-Reload.md) | Make Docker dev loops stable so file changes reload reliably. |
| 4 | [CP-03 Supabase Integration](../07-Coding-Plan/CP-03-Supabase-Integration.md) | Lock down browser/server Supabase boundaries, storage, and Edge Functions. |
| 5 | [CP-04 Project MCP Context](../07-Coding-Plan/CP-04-Project-Mcp-Context.md) | Define project-level MCP context sources, configuration, and runtime loading. |
| 6 | [CP-05 Project Management](../07-Coding-Plan/CP-05-Project-Management.md) | Build project, team, and member CRUD plus project settings shell. |
| 7 | [CP-06 Document Workflow](../07-Coding-Plan/CP-06-Document-Workflow.md) | Build the business logic, tech spec, and coding plan document pipeline. |
| 8 | [CP-07 Workflow Engine UI](../07-Coding-Plan/CP-07-Workflow-Engine-UI.md) | Build workflow builder, run dashboard, approvals, retries, and YOLO flow controls. |
| 9 | [CP-11 Go-Runner Implementation](../07-Coding-Plan/CP-11-Go-Runner-Implementation.md) | Implement the basic runner once workflow run/step state exists. |
| 10 | [CP-09 AI Orchestration](../07-Coding-Plan/CP-09-AI-Orchestration.md) + [CP-12 Artifact Memory RAG](../07-Coding-Plan/CP-12-Artifact-Memory-RAG.md) | Add AI prompt templates, execution logs, artifacts, embeddings, working memory, vector search, and prompt context. |
| 11 | [CP-08 Master Schedule Tasks](../07-Coding-Plan/CP-08-Master-Schedule-Tasks.md) | Build master schedule generation and task board after workflow/document basics are stable. |
| 12 | [CP-10 Integrations Hardening](../07-Coding-Plan/CP-10-Integrations-Hardening.md) | Add integrations, audit hardening, RLS review, and production polish. |

## 3. Guardrails

- Do not start CP-12 before CP-09 has `generate-embedding` and artifact records ready.
- Do not start full CP-11 execution before CP-07 has workflow run and workflow step state in place.
- CP-11 can start in basic mode after CP-07 schema exists, then expand after CP-12 adds prompt memory.
- CP-10 is last, but basic RLS and authorization checks should be reviewed during every database migration.

## 4. Practical Milestones

| Milestone | Plans | Outcome |
|---|---|---|
| Frontend foundation | R1, CP-01 | Admin Web runs on Vite + TanStack. |
| Platform foundations | CP-02, CP-03, CP-04 | Docker reload, Supabase boundaries, and project context wiring are ready. |
| Core product shell | CP-05, CP-06, CP-07 | Users can manage projects, documents, workflows, and approvals. |
| Executable workflows | CP-11 basic runner | Workflow runs can be picked up and executed by the Go-Runner. |
| AI memory layer | CP-09, CP-12 | Artifacts become searchable working memory and selected prompt context. |
| Planning board | CP-08 | Approved plans can become schedules and tasks. |
| Production readiness | CP-10 | Integrations, security, audit, and hardening are complete. |

