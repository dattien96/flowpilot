-- BUG-167: supersedes BUG-166's diagnosis. A live query revealed the real
-- issue was never a *missing* claude-haiku row — ai_supported_models has
-- Claude rows, but under bare ids ("haiku", "sonnet", "opus") instead of the
-- canonical "claude-<name>" prefix that the rest of the system requires:
--   - step_definitions.model literally stores "claude-haiku" (confirmed live).
--   - STEP_MODEL_OPTIONS (apps/admin-web/.../workflow-engine.ts) lists
--     "claude-haiku"/"claude-sonnet"/"claude-opus".
--   - providerKeyFromModel (apps/local-runner/.../provider_registry.go)
--     derives the Claude provider via strings.HasPrefix(model, "claude-") —
--     a bare "haiku"/"sonnet"/"opus" value matches NO provider prefix at all,
--     so a UI path driven straight off ai_supported_models.model_id could
--     silently fail to resolve Claude and fall through to some other
--     provider default.
--
-- Fix in place: rename the bare ids to their prefixed canonical form. Two
-- cases handled so this is safe regardless of whether BUG-166's now-removed
-- migration (which inserted "claude-haiku" etc. as NEW rows alongside the
-- bare ones) ever ran against this database:
--   1. If a prefixed duplicate already exists (from that migration), drop
--      the legacy bare-id row — the prefixed one is canonical and may
--      already carry admin edits (display_name, sort_order, enabled state).
--   2. Otherwise, rename the bare-id row in place so no data (id, source,
--      timestamps) is lost.
delete from public.ai_supported_models legacy
where legacy.provider_key = 'claude'
  and legacy.model_id in ('haiku', 'sonnet', 'opus')
  and exists (
    select 1
    from public.ai_supported_models canonical
    where canonical.provider_key = 'claude'
      and canonical.model_id = 'claude-' || legacy.model_id
  );

update public.ai_supported_models
set model_id = 'claude-' || model_id
where provider_key = 'claude'
  and model_id in ('haiku', 'sonnet', 'opus');
