begin;

update public.projects
set artifact_storage_preference = 'supabase'
where artifact_storage_preference is null
   or btrim(artifact_storage_preference) = '';

alter table public.projects
  alter column artifact_storage_preference set default 'supabase';

alter table public.projects
  alter column artifact_storage_preference set not null;

alter table public.projects
  drop constraint if exists projects_artifact_storage_preference_check;

alter table public.projects
  add constraint projects_artifact_storage_preference_check
  check (artifact_storage_preference in ('supabase', 'google_drive'));

create table if not exists public.artifact_storage_connections (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references public.projects(id) on delete cascade,
  provider text not null check (provider in ('supabase', 'google_drive')),
  status text not null default 'disconnected' check (status in ('disconnected', 'pending', 'connected', 'failed')),
  folder_id text,
  folder_name text,
  oauth_account_email text,
  last_validated_at timestamptz,
  last_error text,
  connected_at timestamptz,
  created_at timestamptz default now(),
  updated_at timestamptz default now(),
  unique (project_id, provider)
);

alter table public.artifact_storage_connections enable row level security;

create policy "authenticated read artifact_storage_connections"
  on public.artifact_storage_connections for select to authenticated
  using (true);

create policy "authenticated write artifact_storage_connections"
  on public.artifact_storage_connections for all to authenticated
  using (true) with check (true);

alter table public.artifact_runs
  add column if not exists storage_provider text default 'supabase',
  add column if not exists remote_object_id text;

update public.artifact_runs
set storage_provider = 'supabase'
where storage_provider is null
   or btrim(storage_provider) = '';

alter table public.artifact_runs
  alter column storage_provider set default 'supabase';

alter table public.artifact_runs
  alter column storage_provider set not null;

alter table public.artifact_runs
  drop constraint if exists artifact_runs_storage_provider_check;

alter table public.artifact_runs
  add constraint artifact_runs_storage_provider_check
  check (storage_provider in ('supabase', 'google_drive'));

create index if not exists artifact_runs_storage_provider_idx
  on public.artifact_runs(storage_provider);

create index if not exists artifact_runs_remote_object_id_idx
  on public.artifact_runs(remote_object_id);

insert into storage.buckets (id, name, public)
values ('flowpilot-artifacts', 'flowpilot-artifacts', false)
on conflict (id) do update
set name = excluded.name,
    public = excluded.public;

drop policy if exists "authenticated read flowpilot artifacts" on storage.objects;
drop policy if exists "authenticated write flowpilot artifacts" on storage.objects;
drop policy if exists "authenticated update flowpilot artifacts" on storage.objects;
drop policy if exists "authenticated delete flowpilot artifacts" on storage.objects;

create policy "authenticated read flowpilot artifacts"
  on storage.objects for select to authenticated
  using (bucket_id = 'flowpilot-artifacts');

create policy "authenticated write flowpilot artifacts"
  on storage.objects for insert to authenticated
  with check (bucket_id = 'flowpilot-artifacts');

create policy "authenticated update flowpilot artifacts"
  on storage.objects for update to authenticated
  using (bucket_id = 'flowpilot-artifacts')
  with check (bucket_id = 'flowpilot-artifacts');

create policy "authenticated delete flowpilot artifacts"
  on storage.objects for delete to authenticated
  using (bucket_id = 'flowpilot-artifacts');

commit;
