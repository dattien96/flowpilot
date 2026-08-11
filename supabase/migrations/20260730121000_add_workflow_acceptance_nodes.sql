-- Migration: Add acceptance_nodes_json to workflows (CP-55 P-1, Task-263)
--
-- Persists agentpack.FlowDefinition.AcceptanceNodes (root YAML key
-- `acceptance_nodes`, added alongside the agent.code/contract.freeze
-- topology validator) through the Supabase-backed user-flow store, so a
-- flow's declared acceptance boundary survives a builtin mirror sync, a
-- clone, a user edit, and a fetch/reload — not just an in-memory load from
-- the embedded pack. Additive only: every existing row defaults to the
-- empty list, matching every pre-CP-55 flow declaring no acceptance_nodes.
alter table workflows
  add column if not exists acceptance_nodes_json jsonb not null default '[]'::jsonb;
