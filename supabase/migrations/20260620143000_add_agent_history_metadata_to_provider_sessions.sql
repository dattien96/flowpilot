alter table workflow_provider_sessions
  add column if not exists parent_run_id uuid references workflow_runs(id) on delete cascade,
  add column if not exists agent_name text,
  add column if not exists agent_role text,
  add column if not exists agent_status text;

create index if not exists workflow_provider_sessions_parent_run_idx
  on workflow_provider_sessions(parent_run_id);
