-- BUG-166: a live query confirmed 'claude-haiku' is entirely absent from
-- ai_supported_models on at least one deployment, even though the original
-- seed migration (20260529153000_add_ai_supported_models.sql) included it.
-- Whatever the reason (that migration never ran on this instance, or the row
-- was removed later), the practical effect is the same: the desktop Step
-- Definitions editor's Model <select> has no <option value="claude-haiku">,
-- so a step whose step_definitions.model is genuinely "claude-haiku" (e.g.
-- the Coder/Reviewer catalog rows from BUG-162) renders with no matching
-- option selected — the browser falls back to showing the first option in
-- the list, even though the underlying stored value is correct.
--
-- Re-insert the three Claude seed rows defensively (all three, not just the
-- confirmed-missing one, since they share whatever root cause dropped
-- claude-haiku). on conflict do nothing: never overwrites a row an admin may
-- have already edited via the "Add model"/edit UI (Task-013/Task-054).
insert into public.ai_supported_models (provider_key, model_id, display_name, source, sort_order) values
  ('claude', 'claude-haiku', 'Claude Haiku', 'seed', 9),
  ('claude', 'claude-sonnet', 'Claude Sonnet', 'seed', 10),
  ('claude', 'claude-opus', 'Claude Opus', 'seed', 11)
on conflict (model_id) do nothing;
