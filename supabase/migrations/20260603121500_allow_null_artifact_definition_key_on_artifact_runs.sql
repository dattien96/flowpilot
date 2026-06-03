begin;

alter table public.artifact_runs
  alter column artifact_definition_key drop not null;

commit;
