-- Migration: Clear node identity from generic flow dispatch-category rows (BUG-241).
--
-- Domain contract (BUG-236): FLOW = workflow, NODE = step. The generic seed
-- rows in step_definitions — 'flow-agent-delegate', 'flow-agent-delegate-coder',
-- 'flow-agent-delegate-reviewer', 'flow-hub-inline', 'flow-context-produce',
-- 'flow-command-validate', 'flow-artifact-audit-draft', etc. — exist ONLY as
-- dispatch-category FK targets for workflow_steps.step_type. They must NOT carry
-- any per-node identity; a flow node's real definition lives on its own
-- node-specific step_definitions row (step_type synthesized by
-- flowNodeStepType, e.g. 'flowpilot_core_flow_pack_review_loop_coder').
--
-- An early BUG-236 draft migration wrongly copied node fields (node_id,
-- behavior_id, agent_ref, …) onto these shared generic rows. The BUG-239
-- repair migration created the correct per-node rows and repointed
-- workflow_steps.step_type to them, but never cleared the stale node identity
-- from the generic rows. That residual contamination broke runtime model
-- resolution: resolveConfiguredModelForAgent scans step_definitions ordered by
-- name.asc and matched a node_id against a contaminated generic row ("Flow: …",
-- which sorts before the real "<pack>: …" per-node row) instead of the node's
-- own row. Two graph nodes sharing an agent file (review-loop's
-- reviewer_correctness / reviewer_security) then resolved to DIFFERENT models,
-- and a flow's main/entry run model disagreed with its coder child for the
-- same node.
--
-- This migration restores the generic rows to pure dispatch categories by
-- nulling their node-definition fields back to the seed defaults. It is
-- idempotent and only touches rows whose step_type begins with the literal
-- 'flow-' prefix (the hand-seeded generic categories). Real per-node mirror
-- rows never begin with 'flow-' because sanitizeStepType rewrites '-' to '_',
-- so they are never matched here. Configured model / yolo_mode / reasoning are
-- deliberately left untouched — the role rows legitimately keep a model for the
-- 'flow-agent-delegate-<role>' role-fallback used by manually-built workflows.
update public.step_definitions
set
  node_id             = null,
  node_lifecycle      = null,
  behavior_id         = null,
  agent_ref           = null,
  depends_on_json     = '[]'::jsonb,
  join_mode           = null,
  cohort              = null,
  prompt_template_ref = null,
  context_ref         = null,
  inputs_json         = '{}'::jsonb,
  outputs_json        = '{}'::jsonb
where step_type like 'flow-%'
  and (
    node_id is not null
    or node_lifecycle is not null
    or behavior_id is not null
    or agent_ref is not null
    or depends_on_json <> '[]'::jsonb
    or join_mode is not null
    or cohort is not null
    or prompt_template_ref is not null
    or context_ref is not null
    or inputs_json <> '{}'::jsonb
    or outputs_json <> '{}'::jsonb
  );
