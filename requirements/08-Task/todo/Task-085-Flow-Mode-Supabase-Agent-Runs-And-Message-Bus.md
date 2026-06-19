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
- Add `agent_runs` and `agent_messages` tables and `workflow_runs.parent_run_id` via additive, backward-compatible migrations; reuse `workflow_provider_sessions`/`workflow_provider_events` per agent run.
- Let a workflow step run as an agent with a reviewer gate, reusing the Task-084 orchestrator contract.

### Current Ask

- Implement Phase 2 persistence + workflow-engine integration only after the Phase 1 orchestrator/bus contract (Task-084) is stable.

### Key Decisions

- `T-1` `agent_runs` carries `role`, `depends_on jsonb`, `status`, `parent_run_id`, `workflow_run_id` (nullable for pure-chat agents).
- `T-2` `agent_messages` is the durable bus (`from/to_agent_run_id`, `kind`, `payload_json`, `artifact_id`, `consumed_at`).
- `T-3` Reuse `workflow_provider_sessions`/`events` per agent run for streaming + cross-PC resume; reconstruct the tree from `agent_runs`.
- `T-4` Migrations are additive; single-agent and existing workflow runs are unaffected.

### Constraints

- Run GitNexus impact analysis before editing the workflow engine/runtime symbols; warn on HIGH/CRITICAL.
- Migrations must be backward compatible and reversible; no backfill that mutates existing runs.
- A dedicated upstream SD/SS for multi-agent should be resolved (CP-19 open question) before finalizing the schema.

### Open Questions

- Should `agent_runs` be a new table or columns added directly to `workflow_runs`? (Current decision: new table + `parent_run_id` on `workflow_runs`.)
- Cross-PC resume policy when a child provider session cannot be re-established.

### Source Refs

- `CP-19` P-6; `supabase/migrations/`; `workflow_orchestrator.go`; `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`.

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

- `T-1` Additive migration: `agent_runs`, `agent_messages`, `ALTER TABLE workflow_runs ADD parent_run_id`.
- `T-2` Persist orchestrator state (runs, edges, bus messages) to these tables; reconstruct the tree on resume.
- `T-3` Wire `workflow-start-runtime` so a workflow step can run as an agent with a reviewer gate, reusing the Task-084 orchestrator.
- `T-4` Reuse `workflow_provider_sessions`/`workflow_provider_events` keyed per agent run.

## 5. Touched Areas

- files: `supabase/migrations/<new>.sql`, `apps/local-runner/internal/runner/agent_orchestrator.go`, `workflow_orchestrator.go`, `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`, related entity types
- modules: workflow engine; local-runner orchestration; persistence
- routes: workflow-engine run/resume paths
- tables: `agent_runs` (new), `agent_messages` (new), `workflow_runs` (add `parent_run_id`)

## 6. Acceptance Check

- A multi-agent run persists to `agent_runs`/`agent_messages` and resumes (tree reconstructed) after a runner restart.
- A workflow step runs as an agent with a reviewer gate and completes the feedback loop in flow mode.
- Existing single-agent runs and workflows are unaffected; migrations apply and revert cleanly.

## 7. Out of Scope

- New UI beyond what Tasks 083–084 deliver; provider adapter changes.

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
