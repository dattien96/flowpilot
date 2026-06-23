# Task-085: Flow-Mode Supabase Agent Runs And Message Bus

## Metadata

- Document ID: `Task-085`
- Title: `Flow-Mode Supabase Agent Runs And Message Bus`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [Task-084: Dependency Feedback Loop And Orchestration Board](./Task-084-Dependency-Feedback-Loop-And-Orchestration-Board.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- Replaces: `None`
- Tags: `multi-agent, flow-mode, supabase, workflow-engine, migration, persistence`

## AI Quick View

### Summary

- Formalize the Phase 1 in-memory agent graph + bus as durable Supabase rows so multi-agent runs persist, resume after a runner restart, and integrate with the workflow engine (flow mode).
- Add `agent_runs` and `agent_messages` tables plus `workflow_runs.parent_run_id` via additive, backward-compatible migrations; keep existing `workflow_provider_sessions`/`workflow_provider_events` keyed by `workflow_run_id`.
- Let a workflow step run as an agent with a reviewer gate, reusing the Task-084 `AgentGraphSnapshot` / bus contract without changing the provider-adapter interface.

### Current Ask

- Implement Phase 2 persistence + workflow-engine integration only after the Phase 1 orchestrator/bus contract (Task-084) is stable and its DTOs are treated as the runner/client contract.

### Key Decisions

- `T-1` `agent_runs` is a new table, not extra columns directly on `workflow_runs`. For flow-mode agent runs, `agent_runs.workflow_run_id` is unique and points at the provider-bearing `workflow_runs` row; `workflow_runs.parent_run_id` links child workflow runs back to the parent workflow run for existing provider-session joins.
- `T-2` `agent_messages` is the durable bus (`from_agent_run_id`, `to_agent_run_id`, `kind`, `payload_json`, `artifact_id`, `consumed_at`) and mirrors Task-084 bus message kinds, including `ready-for-review`, `changes-requested`, `approved`, `rejected`, `handoff`, and `user-feedback`.
- `T-3` Reuse `workflow_provider_sessions`/`workflow_provider_events` per agent by writing them against the child agent's `workflow_run_id`; do not add `agent_run_id` to provider telemetry unless a later design proves it is required.
- `T-4` Reconstruct `AgentGraphSnapshot` from `agent_runs` + `agent_messages` + existing provider session/event rows on runner restart or workflow resume.
- `T-5` Migrations are additive; existing single-agent runs get `workflow_runs.parent_run_id = null`, no backfill mutation, and no behavior change.

### Constraints

- Run GitNexus impact analysis before editing the workflow engine/runtime symbols; warn on HIGH/CRITICAL.
- Migrations must be backward compatible, idempotent, and reversible in local dev; no backfill that mutates existing runs.
- A dedicated upstream SD/SS for multi-agent should be resolved (CP-19 open question) before finalizing the schema.
- Do not change `ProviderRuntimeAdapter`, `ProviderEvent` / `ProviderEventDTO`, or SSE semantics except for additive Task-084 event variants already defined.
- Preserve YOLO=false approval/question behavior: durable agent replay must not auto-approve, auto-answer, or hide child gates.
- Flow-mode persistence must not replace Phase 1 chat/Drive sync behavior; chat-mode multi-agent trees continue to use the existing local/Drive path unless a later task explicitly migrates chat to Supabase.

### Open Questions

- Cross-PC resume policy when a child provider session cannot be re-established.
- Whether pure-chat multi-agent runs should ever be copied into `agent_runs`; this task scopes durable Supabase persistence to flow-mode runs only.

### Source Refs

- `CP-19` P-6; Task-084 `AgentGraphSnapshot`, `AgentBusMessage`, loop control contract; `supabase/migrations/20260615120000_add_workflow_provider_tables.sql`; `supabase/migrations/20260520033000_cp07_workflow_engine.sql`; `workflow_orchestrator.go`; `apps/local-runner/internal/runner/supabase_workflow_store.go`; `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`.

## 1. Goal

Persist and resume multi-agent runs and integrate them into the workflow engine, so flow-mode workflows can use agent steps with reviewer gates and survive restarts and cross-PC moves.

## 2. Parent Links

- coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `CP-19` P-6

## 3. Trigger

CP-19 Phase 2 requires the agent graph/bus to become durable and workflow-integrated; Phase 1 keeps it in memory only.

## 4. Exact Change

- `T-1` Additive migration:
  - `ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS parent_run_id uuid REFERENCES workflow_runs(id) ON DELETE SET NULL`
  - `agent_runs` with `id`, `workflow_run_id`, `parent_agent_run_id`, `root_workflow_run_id`, `project_id`, `workflow_id`, `agent_name`, `role`, `provider`, `model`, `status`, `depends_on jsonb`, `round_index`, `round_cap`, `spawned_by`, `created_at`, `closed_at`, `summary`
  - `agent_messages` with `id`, `root_workflow_run_id`, `from_agent_run_id`, `to_agent_run_id`, `kind`, `payload_json`, `artifact_id`, `consumed_at`, `created_at`
- `T-2` Add indexes and constraints: `agent_runs.workflow_run_id` unique when non-null; indexes on `root_workflow_run_id`, `parent_agent_run_id`, `status`, and message `root_workflow_run_id/created_at`; status/kind checks match the Task-084 contract.
- `T-3` Add RLS policies that mirror workflow run access through `project_teams` / `team_members`; service-role/local-runner writes continue to work, and authenticated reads only see rows for accessible projects.
- `T-4` Extend the Supabase workflow store with idempotent agent graph methods: upsert/list `agent_runs`, append/list `agent_messages`, mark messages consumed, and rebuild `AgentGraphSnapshot`.
- `T-5` Persist Task-084 graph/bus transitions to Supabase. Bus appends are append-only; graph/run status updates are idempotent patches keyed by run/message IDs.
- `T-6` Wire `workflow-start-runtime` so a workflow step can spawn durable child workflow runs for coder/reviewer agents, create matching `agent_runs`, and run the reviewer gate through the Task-084 orchestrator.
- `T-7` Reuse `workflow_provider_sessions`/`workflow_provider_events` by creating/upserting them with each child agent's `workflow_run_id`. Existing provider telemetry tables remain backward compatible.
- `T-8` Reconstruct the tree and bus from Supabase after local-runner restart or cross-PC resume; if a provider session cannot be re-established, mark only that child `failed` / `needs_attention` and keep the parent graph inspectable.

## 5. Touched Areas

- files: `supabase/migrations/<new>.sql`, `apps/local-runner/internal/runner/agent_orchestrator.go`, `workflow_store.go`, `supabase_workflow_store.go`, `workflow_orchestrator.go`, `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`, related TypeScript/Go entity types and tests
- modules: workflow engine; local-runner orchestration; Supabase persistence; workflow resume/history
- routes: workflow-engine run/resume paths; Task-084 graph/bus HTTP reads must be backed by Supabase for flow-mode runs
- tables: `agent_runs` (new), `agent_messages` (new), `workflow_runs` (add nullable `parent_run_id`), existing `workflow_provider_sessions` / `workflow_provider_events` reused unchanged

## 6. Acceptance Check

- A multi-agent run persists to `agent_runs`/`agent_messages` and resumes (tree reconstructed) after a runner restart.
- A workflow step runs as an agent with a reviewer gate and completes the feedback loop in flow mode.
- Provider sessions/events for each child are written with that child's `workflow_run_id`; existing single-agent provider session/event queries still work.
- RLS permits authenticated users to read only agent rows for projects they can access; local-runner/service-role writes are not blocked.
- Existing single-agent runs and workflows are unaffected; migrations apply cleanly on an existing database and can be reverted locally without data loss to pre-existing tables.
- Resume failure for one child provider session leaves the graph inspectable and marks that child as failed/needs-attention without corrupting the parent run.
- Tests cover migration shape/constraints, Supabase store graph round-trip, message append/consume idempotency, workflow runtime child-run creation, restart reconstruction, and single-agent regression.

## 7. Out of Scope

- New UI beyond what Tasks 083–084 deliver; provider adapter changes; durable Supabase persistence for normal chat/Drive-synced Phase 1 runs; changing Task-084 Graph/DAG UX; broad workflow-engine rewrites unrelated to agent steps.

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
