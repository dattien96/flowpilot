-- Migration: Add workflow_run_sessions table

create table if not exists workflow_run_sessions (
  id uuid primary key default gen_random_uuid(),
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  provider text not null,
  model text not null,
  transport_type text not null,
  provider_session_id text,
  process_key text,
  status text not null default 'active',
  metadata_json jsonb default '{}'::jsonb,
  started_at timestamptz default now(),
  completed_at timestamptz
);

-- Enable RLS
alter table workflow_run_sessions enable row level security;

-- Policies for Authenticated Users (Admins)
create policy "authenticated read workflow_run_sessions"
  on workflow_run_sessions for select to authenticated using (true);

create policy "authenticated write workflow_run_sessions"
  on workflow_run_sessions for all to authenticated using (true) with check (true);

-- Create index for fast query
create index if not exists workflow_run_sessions_run_idx on workflow_run_sessions(workflow_run_id);
