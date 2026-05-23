alter table if exists step_definitions
  add column if not exists team_role text,
  add column if not exists subagent text,
  add column if not exists model text;
