-- Migration: Add context_sources to step_definitions (CP-44 / Task-196).
--
-- Mirrors required_mcps: a step-level, user-selectable list of enabled
-- context-source ids for that step's Plan-time context harness (CP-44's
-- ContextSourceRegistry). Empty/default means "fall back to the flow-level
-- contexts.sources binding (Task-194), then the runner's default built-in
-- set" — existing rows are unaffected.
--
-- No new table: SD-22 explicitly avoids adding Supabase tables for this
-- feature; this is a narrow additive column on the existing step_definitions
-- catalog, same shape as the pre-existing required_mcps column.

alter table public.step_definitions
  add column if not exists context_sources jsonb not null default '[]'::jsonb;
