-- BUG-164: workflow_steps is a relationship table (workflow <-> step type,
-- with ordering) — a step's model/provider/reasoning is entirely determined
-- by its step type's own catalog row (step_definitions.model), not by a
-- per-workflow-instance override. Per SS-05/SD-06, the only real resolution
-- levels are project default -> workflow default (workflows.model_override,
-- unchanged by this migration) -> step-type catalog default
-- (step_definitions.model). workflow_steps never needed its own override
-- layer between those; it existed only because the classic CP-07 step model
-- allowed one, and in practice it caused repeated data drift (BUG-160/161/
-- 162/163: forced defaults, stale overrides, a schema-default 'codex' value
-- silently winning over the real configured model).
--
-- Dropping these columns removes that entire class of drift permanently: a
-- step's provider/model is now always exactly what its step type says it is.
alter table public.workflow_steps
  drop column if exists provider_override,
  drop column if exists model_override,
  drop column if exists reasoning_effort_override;
