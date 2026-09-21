-- CP-70 Task-402: allow devin rows in ai_supported_models.
--
-- The Desktop "Detect models" sync (supabaseAdminRepository.createSupportedModel)
-- inserts provider_key='devin' rows, but the column CHECK still enumerated
-- only the five earlier providers, so every devin model sync failed with:
--   new row for relation "ai_supported_models" violates check constraint
--   "ai_supported_models_provider_key_check"
-- (Same class as CA-689 for opencode — provider_key is CHECK-constrained, not
-- opaque text.)
--
-- Verify the current definition first if unsure:
--   select conname, pg_get_constraintdef(oid)
--   from pg_constraint where conname = 'ai_supported_models_provider_key_check';
--
-- Idempotent (CA-690 review lesson): the whole drop+add only fires while the
-- constraint still lacks 'devin'; a re-run against an already-migrated DB is
-- a no-op.

do $$
begin
  if not exists (
    select 1
    from pg_constraint c
    join pg_class t on t.oid = c.conrelid
    where c.conname = 'ai_supported_models_provider_key_check'
      and t.relname = 'ai_supported_models'
      and pg_get_constraintdef(c.oid) like '%devin%'
  ) then
    alter table ai_supported_models
      drop constraint if exists ai_supported_models_provider_key_check;
    alter table ai_supported_models
      add constraint ai_supported_models_provider_key_check
      check (provider_key in ('codex', 'claude', 'gemini', 'grok', 'opencode', 'devin'));
  end if;
end $$;
