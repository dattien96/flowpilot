-- CA-689 / CP-57 Task-302: allow opencode rows in ai_supported_models.
--
-- The Desktop "Detect models" sync (supabaseAdminRepository.insertSupportedModel)
-- inserts provider_key='opencode' rows, but the column CHECK still enumerated
-- only the original four providers, so every opencode model sync failed with:
--   new row for relation "ai_supported_models" violates check constraint
--   "ai_supported_models_provider_key_check"
-- (CP-57 §6 incorrectly assumed provider_key was opaque text — it is checked.)
--
-- Live DB census before this migration: codex=8, claude=4, gemini=5, grok=4.
--
-- Verify the current definition first if unsure:
--   select conname, pg_get_constraintdef(oid)
--   from pg_constraint where conname = 'ai_supported_models_provider_key_check';
--
-- Idempotent: the drop only fires while the constraint still lacks 'opencode'.

do $$
begin
  if exists (
    select 1
    from pg_constraint c
    join pg_class t on t.oid = c.conrelid
    where c.conname = 'ai_supported_models_provider_key_check'
      and t.relname = 'ai_supported_models'
      and pg_get_constraintdef(c.oid) not like '%opencode%'
  ) then
    alter table ai_supported_models
      drop constraint ai_supported_models_provider_key_check;
  end if;
end $$;

alter table ai_supported_models
  add constraint ai_supported_models_provider_key_check
  check (provider_key in ('codex', 'claude', 'gemini', 'grok', 'opencode'));
