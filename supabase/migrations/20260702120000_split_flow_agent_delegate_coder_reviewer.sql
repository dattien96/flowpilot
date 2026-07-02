-- BUG-161: the CP-42 migration (20260701090000_add_flow_engine_attrs_to_workflows.sql)
-- seeded exactly one generic 'flow-agent-delegate' step_definitions row as the
-- shared dispatch category for every agent.delegate flow node (coder,
-- reviewer_correctness, reviewer_security, rag-harness's implement, ...).
-- That is correct for BUILT-IN flow-pack mirrors, whose real per-node identity
-- lives in workflow_steps.node_id/agent_ref (BUG-155). But a user manually
-- building a custom workflow in Settings > Workflows picks a step TYPE from
-- the "Add step" dropdown, which only ever offered the one generic name — so
-- a hand-built workflow could not distinguish a "coder" step from a
-- "reviewer" step at the point of adding it.
--
-- Add two additional, purpose-named step_definitions rows so the manual
-- workflow builder has a real choice, without touching the generic row
-- (still needed by flows that don't cleanly split into coder/reviewer, e.g.
-- rag-harness's "implement" node, and by any workflow_steps row this
-- migration's best-effort reassignment below can't confidently classify).
insert into public.step_definitions (step_type, name, description, agent_type)
values
  (
    'flow-agent-delegate-coder',
    'Flow: Coder',
    'Flow-engine node that delegates to the coder agent. Same runtime contract as flow-agent-delegate (behaviorHandler behaviorAgentDelegate); this is a purpose-named catalog entry for the manual workflow builder.',
    'standard'
  ),
  (
    'flow-agent-delegate-reviewer',
    'Flow: Reviewer',
    'Flow-engine node that delegates to a reviewer agent. Same runtime contract as flow-agent-delegate (behaviorHandler behaviorAgentDelegate); this is a purpose-named catalog entry for the manual workflow builder.',
    'standard'
  )
on conflict (step_type) do nothing;

-- Best-effort reassignment of existing workflow_steps rows: a row already
-- carrying an agent_ref that clearly names "coder" or "review" is
-- reclassified to the matching new step_type. Anything ambiguous (no
-- agent_ref, or an agent_ref that names neither) is left pointing at the
-- original generic 'flow-agent-delegate' row rather than guessed at.
update public.workflow_steps
set step_type = 'flow-agent-delegate-coder'
where step_type = 'flow-agent-delegate'
  and agent_ref is not null
  and agent_ref ilike '%coder%';

update public.workflow_steps
set step_type = 'flow-agent-delegate-reviewer'
where step_type = 'flow-agent-delegate'
  and agent_ref is not null
  and agent_ref ilike '%review%';
