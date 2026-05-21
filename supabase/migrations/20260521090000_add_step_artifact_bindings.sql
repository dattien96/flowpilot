alter table if exists step_definitions
  add column if not exists input_artifact_definitions jsonb not null default '[]'::jsonb,
  add column if not exists output_artifact_definitions jsonb not null default '[]'::jsonb;
