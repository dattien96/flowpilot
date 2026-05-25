alter table if exists step_definitions
  add column if not exists reasoning_effort text;
