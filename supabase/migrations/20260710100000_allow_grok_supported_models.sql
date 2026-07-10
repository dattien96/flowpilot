-- Task-213: allow 'grok' as a provider_key in ai_supported_models.
--
-- The table's original CHECK constraint (20260529153000_add_ai_supported_models.sql)
-- only allowed codex/claude/gemini. Grok Build (CP-46) is now a first-class
-- provider, and Task-213's model auto-detect/sync needs to insert grok rows —
-- both currently fail with a constraint violation. Idempotent: drop-if-exists
-- then re-add with the widened list; no row changes.

alter table public.ai_supported_models
  drop constraint if exists ai_supported_models_provider_key_check;

alter table public.ai_supported_models
  add constraint ai_supported_models_provider_key_check
  check (provider_key in ('codex', 'claude', 'gemini', 'grok'));
