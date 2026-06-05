begin;

create table if not exists public.artifact_run_replicas (
  id uuid primary key default gen_random_uuid(),
  artifact_run_id uuid not null references public.artifact_runs(id) on delete cascade,
  project_id uuid not null references public.projects(id) on delete cascade,
  provider text not null check (provider in ('supabase', 'google_drive')),
  storage_scope_key text,
  remote_path text not null default '',
  remote_object_id text,
  sync_status text not null default 'queued' check (sync_status in ('queued', 'syncing', 'synced', 'failed')),
  checksum text,
  last_synced_at timestamptz,
  last_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (artifact_run_id, provider)
);

create index if not exists artifact_run_replicas_project_provider_idx
  on public.artifact_run_replicas(project_id, provider);

create index if not exists artifact_run_replicas_project_provider_scope_idx
  on public.artifact_run_replicas(project_id, provider, storage_scope_key);

create index if not exists artifact_run_replicas_artifact_idx
  on public.artifact_run_replicas(artifact_run_id);

create index if not exists artifact_run_replicas_remote_object_id_idx
  on public.artifact_run_replicas(remote_object_id);

alter table public.artifact_run_replicas enable row level security;

drop policy if exists "authenticated read artifact_run_replicas" on public.artifact_run_replicas;
create policy "authenticated read artifact_run_replicas"
  on public.artifact_run_replicas for select to authenticated
  using (true);

drop policy if exists "authenticated write artifact_run_replicas" on public.artifact_run_replicas;
create policy "authenticated write artifact_run_replicas"
  on public.artifact_run_replicas for all to authenticated
  using (true) with check (true);

insert into public.artifact_run_replicas (
  artifact_run_id,
  project_id,
  provider,
  storage_scope_key,
  remote_path,
  remote_object_id,
  sync_status,
  last_synced_at,
  created_at,
  updated_at
)
select
  ar.id,
  ar.project_id,
  ar.storage_provider,
  case
    when ar.storage_provider = 'google_drive' and btrim(coalesce(conn.folder_id, '')) <> '' then 'gdrive:' || btrim(conn.folder_id)
    else null
  end,
  ar.remote_path,
  nullif(btrim(ar.remote_object_id), ''),
  case
    when ar.sync_status in ('queued', 'syncing', 'synced', 'failed') then ar.sync_status
    else 'queued'
  end,
  case
    when ar.sync_status = 'synced' then ar.updated_at
    else null
  end,
  ar.created_at,
  ar.updated_at
from public.artifact_runs ar
left join public.artifact_storage_connections conn
  on conn.project_id = ar.project_id
 and conn.provider = 'google_drive'
where ar.project_id is not null
  and btrim(coalesce(ar.remote_path, '')) <> ''
  and btrim(coalesce(ar.storage_provider, '')) in ('supabase', 'google_drive')
on conflict (artifact_run_id, provider) do update
set storage_scope_key = coalesce(excluded.storage_scope_key, public.artifact_run_replicas.storage_scope_key),
    remote_path = excluded.remote_path,
    remote_object_id = excluded.remote_object_id,
    sync_status = excluded.sync_status,
    last_synced_at = excluded.last_synced_at,
    updated_at = excluded.updated_at;

commit;
