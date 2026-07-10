-- Task-215: capture per-model reasoning-effort support and context-window
-- size on ai_supported_models.
--
-- Both Codex (`codex debug models` -> supported_reasoning_levels/
-- default_reasoning_level/context_window/max_context_window) and Grok
-- (~/.grok/models_cache.json -> reasoning_efforts/reasoning_effort/
-- context_window) already return this data in the exact same detection
-- calls Task-213 made -- the runner just wasn't reading the fields. This
-- migration only adds columns for the runner/desktop to populate; it does
-- not touch existing rows.

alter table public.ai_supported_models
  add column if not exists supported_reasoning_efforts jsonb null,
  add column if not exists default_reasoning_effort text null,
  add column if not exists context_window_tokens integer null,
  add column if not exists max_context_window_tokens integer null;
