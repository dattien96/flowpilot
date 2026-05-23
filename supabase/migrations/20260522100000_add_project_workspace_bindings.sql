create table if not exists project_workspace_bindings (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  local_path text not null,
  label text,
  created_at timestamptz default now(),
  updated_at timestamptz default now(),
  unique (project_id, local_path)
);

create index if not exists project_workspace_bindings_project_id_idx
  on project_workspace_bindings(project_id);

create index if not exists project_workspace_bindings_local_path_idx
  on project_workspace_bindings(local_path);

insert into project_workspace_bindings (id, project_id, local_path, label)
select
  gen_random_uuid(),
  projects.id,
  btrim(projects.directory_path),
  'Primary'
from projects
where projects.directory_path is not null
  and btrim(projects.directory_path) <> ''
on conflict (project_id, local_path) do nothing;

alter table project_workspace_bindings enable row level security;

drop policy if exists "authenticated read project workspace bindings" on project_workspace_bindings;
create policy "authenticated read project workspace bindings"
  on project_workspace_bindings for select
  to authenticated
  using (true);

drop policy if exists "authenticated write project workspace bindings" on project_workspace_bindings;
create policy "authenticated write project workspace bindings"
  on project_workspace_bindings for all
  to authenticated
  using (true)
  with check (true);
