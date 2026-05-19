create extension if not exists pgcrypto;

create table if not exists integrations (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  type text not null check (type in ('jira', 'figma', 'google_drive', 'firebase', 'telegram')),
  label text not null default '',
  config_encrypted jsonb not null default '{}'::jsonb,
  status text not null default 'pending' check (status in ('pending', 'awaiting_oauth', 'connected', 'failed')),
  last_synced_at timestamptz,
  last_error text,
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

create index if not exists integrations_project_idx on integrations(project_id);
create index if not exists integrations_project_type_idx on integrations(project_id, type);

alter table integrations enable row level security;

drop policy if exists integrations_select_all_authenticated on integrations;
create policy "integrations_select_all_authenticated"
  on integrations for select
  using (auth.role() = 'authenticated');

drop policy if exists integrations_write_all_authenticated on integrations;
create policy "integrations_write_all_authenticated"
  on integrations for all
  using (auth.role() = 'authenticated')
  with check (auth.role() = 'authenticated');
