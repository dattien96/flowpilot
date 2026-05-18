# Task-01: Execution Order

This document defines the recommended execution order for the FlowPilot refactor and feature buildout.

## 1. Recommended Order

```text
R1 -> CP-01 -> CP-02 -> CP-03 -> CP-04 -> CP-08 basic runner -> CP-06 + CP-09 -> CP-05 -> CP-07
```

## 2. Step Details

| Order | Plan | Purpose |
|---|---|---|
| 1 | [R1 Refactor To React](../10-Refactor/R1-Refactor-To-React-Plan.md) | Migrate `apps/admin-web` from Next.js to Vite + TanStack. |
| 2 | [CP-01 Foundation Setup](../07-Coding-Plan/CP-01-Foundation-Setup.md) | Validate the new frontend foundation: Vite, routing, auth, layout, and query setup. |
| 3 | [CP-02 Project Management](../07-Coding-Plan/CP-02-Project-Management.md) | Build project, team, member, and project context basics. |
| 4 | [CP-03 Document Workflow](../07-Coding-Plan/CP-03-Document-Workflow.md) | Build the business logic, tech spec, and coding plan document pipeline. |
| 5 | [CP-04 Workflow Engine UI](../07-Coding-Plan/CP-04-Workflow-Engine-UI.md) | Build workflow builder, run dashboard, approvals, retries, and YOLO flow controls. |
| 6 | [CP-08 Go-Runner Implementation](../07-Coding-Plan/CP-08-Go-Runner-Implementation.md) | Implement the basic runner once workflow run/step state exists. |
| 7 | [CP-06 AI Orchestration](../07-Coding-Plan/CP-06-AI-Orchestration.md) + [CP-09 Artifact Memory RAG](../07-Coding-Plan/CP-09-Artifact-Memory-RAG.md) | Add AI prompt templates, execution logs, artifacts, embeddings, working memory, vector search, and prompt context. |
| 8 | [CP-05 Master Schedule Tasks](../07-Coding-Plan/CP-05-Master-Schedule-Tasks.md) | Build master schedule generation and task board after workflow/document basics are stable. |
| 9 | [CP-07 Integrations Hardening](../07-Coding-Plan/CP-07-Integrations-Hardening.md) | Add integrations, audit hardening, RLS review, and production polish. |

## 3. Guardrails

- Do not start CP-09 before CP-06 has `generate-embedding` and artifact records ready.
- Do not start full CP-08 execution before CP-04 has workflow run and workflow step state in place.
- CP-08 can start in basic mode after CP-04 schema exists, then expand after CP-09 adds prompt memory.
- CP-07 is last, but basic RLS and authorization checks should be reviewed during every database migration.

## 4. Practical Milestones

| Milestone | Plans | Outcome |
|---|---|---|
| Frontend foundation | R1, CP-01 | Admin Web runs on Vite + TanStack. |
| Core product shell | CP-02, CP-03, CP-04 | Users can manage projects, documents, workflows, and approvals. |
| Executable workflows | CP-08 basic runner | Workflow runs can be picked up and executed by the Go-Runner. |
| AI memory layer | CP-06, CP-09 | Artifacts become searchable working memory and selected prompt context. |
| Planning board | CP-05 | Approved plans can become schedules and tasks. |
| Production readiness | CP-07 | Integrations, security, audit, and hardening are complete. |
