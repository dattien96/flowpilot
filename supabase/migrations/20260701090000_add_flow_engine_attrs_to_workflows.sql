-- Migration: Add flow-engine attributes to workflows/workflow_steps (CP-42 Task-175/179)
--
-- Supersedes an earlier, never-applied `flow_definitions` migration. Rather than run a
-- parallel table, built-in agentpack flows (Review Loop, RAG Harness) are mirrored
-- directly into `workflows` + `workflow_steps` as read-only rows, and the existing
-- Settings "Workflows/Steps" screen (WorkflowsSettings.tsx) is reused instead of a new
-- screen. `workflow_steps.step_type` already references the reusable `step_definitions`
-- catalog; `behavior_id` is added alongside it as the CP-42 behavior-ID a node runs under
-- (agent.delegate/hub.inline/...), decoupled from the human-facing step_type label.

-- ---------------------------------------------------------------------------
-- workflows: built-in/read-only flags, pack identity for mirror-sync staleness
-- checks, and chat-orchestration + cap-policy metadata.
-- ---------------------------------------------------------------------------
alter table workflows
  add column if not exists is_builtin boolean not null default false,
  add column if not exists editable boolean not null default true,
  add column if not exists cloneable boolean not null default true,
  add column if not exists cloned_from uuid references workflows(id) on delete set null,
  add column if not exists pack_id text,
  add column if not exists pack_version text,
  add column if not exists pack_flow_id text,
  add column if not exists pack_hash text,
  add column if not exists selectable_in_json jsonb not null default '[]'::jsonb,
  add column if not exists chat_baseline boolean not null default false,
  add column if not exists chat_sub_modes_json jsonb not null default '[]'::jsonb,
  add column if not exists policy_cap int,
  add column if not exists policy_on_cap text,
  add column if not exists policy_extend_by int,
  add column if not exists policy_extend_max int,
  -- Edge list for the whole flow graph ({from,to,when,kind} objects), a
  -- workflow-level property. Read by the live executor (flow_executor.go) to
  -- auto-spawn forward-edge cohorts and resolve back-edge reinvoke targets.
  add column if not exists edges_json jsonb not null default '[]'::jsonb,
  -- Flow-level named context bindings ({name: {ref: "contexts/....yaml"}}),
  -- e.g. rag-harness's `contexts.main_context`. Preserved with full fidelity
  -- from the source agentpack YAML alongside per-node inputs_json/
  -- outputs_json on workflow_steps below.
  add column if not exists contexts_json jsonb not null default '{}'::jsonb;

-- Mirror-sync lookup by pack identity. Deliberately NOT a partial index
-- (`where is_builtin = true`): PostgREST's `on_conflict=pack_id,pack_flow_id`
-- query param emits a bare `ON CONFLICT (pack_id, pack_flow_id)` with no WHERE
-- clause, and Postgres cannot infer a partial unique index from that — it
-- would fail at insert time with "no unique or exclusion constraint matching
-- the ON CONFLICT specification" (BUG-NOTE-CP42 #2). A plain unique index
-- works without a predicate because non-builtin rows always leave pack_id/
-- pack_flow_id NULL (see supabase_workflow_flow_store.go Upsert), and a
-- standard btree unique index never treats two NULLs as conflicting.
create unique index if not exists workflows_pack_flow_uidx
  on workflows(pack_id, pack_flow_id);

-- ---------------------------------------------------------------------------
-- workflow_steps: per-node flow-graph attributes. node_id is the stable
-- flow-graph identifier for this step within its workflow (e.g. "coder",
-- "reviewer_correctness") — distinct from step_type, which is a reusable,
-- shared step-definition key that multiple nodes across multiple workflows
-- can reference. dependsOn references other nodes' node_id within the same
-- workflow.
-- ---------------------------------------------------------------------------
alter table workflow_steps
  add column if not exists node_id text,
  add column if not exists behavior_id text,
  add column if not exists agent_ref text,
  add column if not exists depends_on_json jsonb not null default '[]'::jsonb,
  add column if not exists join_mode text,
  add column if not exists cohort text,
  add column if not exists prompt_template_ref text,
  add column if not exists context_ref text,
  -- Per-node input/output context bindings ({contextName: artifactTypeRef}),
  -- e.g. rag-harness's "implement" node inputs.main_context ->
  -- flow_context_package.v1. Round-tripped alongside node_id/behavior_id so a
  -- mirrored flow retains the same context/package wiring the source
  -- agentpack YAML declares (BUG-NOTE-CP42 #10).
  add column if not exists inputs_json jsonb not null default '{}'::jsonb,
  add column if not exists outputs_json jsonb not null default '{}'::jsonb;

create unique index if not exists workflow_steps_workflow_node_uidx
  on workflow_steps(workflow_id, node_id)
  where node_id is not null;

-- ---------------------------------------------------------------------------
-- Seed generic step_definitions rows for the CP-42 canonical behavior IDs.
-- workflow_steps.step_type has a NOT NULL FK to step_definitions(step_type),
-- so a mirrored flow-engine node needs a valid step_type to reference even
-- though its real per-node identity lives in node_id/behavior_id/agent_ref.
-- These rows are generic dispatch categories, not human-authored step
-- definitions — a node's specific behavior/agent binding always comes from
-- its own behavior_id/agent_ref columns, never from this row's fields.
-- ---------------------------------------------------------------------------
insert into step_definitions (step_type, name, description, agent_type)
values
  ('flow-agent-delegate', 'Flow: Agent Delegate', 'Generic flow-engine node that delegates to a specific agent, per this step''s node_id/behavior_id/agent_ref.', 'standard'),
  ('flow-hub-inline', 'Flow: Hub Inline', 'Generic flow-engine node that runs deterministic hub/synthesis logic inline, per this step''s behavior_id.', 'standard'),
  ('flow-context-produce', 'Flow: Context Produce', 'Generic flow-engine node that produces a typed context artifact.', 'standard'),
  ('flow-context-render', 'Flow: Context Render', 'Generic flow-engine node that renders a context artifact into a prompt fragment.', 'standard'),
  ('flow-command-validate', 'Flow: Command Validate', 'Generic flow-engine node that validates a command result.', 'standard'),
  ('flow-validation-summarize', 'Flow: Validation Summarize', 'Generic flow-engine node that summarizes validation results.', 'standard'),
  ('flow-artifact-audit-draft', 'Flow: Artifact Audit Draft', 'Generic flow-engine node that prepares a draft audit artifact.', 'standard'),
  ('flow-control', 'Flow: Control', 'Generic flow-engine node that maps a declared tool outcome to the generic flow_control contract.', 'standard'),
  ('flow-user-confirm', 'Flow: User Confirm', 'Generic flow-engine node that gates on explicit user confirmation.', 'standard')
on conflict (step_type) do nothing;
