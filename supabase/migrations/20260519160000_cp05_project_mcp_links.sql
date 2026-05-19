create table if not exists project_mcp_links (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  integration_id uuid not null references integrations(id) on delete cascade,
  type text not null check (type in ('jira', 'figma', 'google_drive', 'firebase', 'telegram')),
  created_at timestamptz default now(),
  updated_at timestamptz default now(),
  unique (project_id, type)
);

create index if not exists project_mcp_links_project_idx on project_mcp_links(project_id);
create index if not exists project_mcp_links_integration_idx on project_mcp_links(integration_id);

insert into project_mcp_links (project_id, integration_id, type)
select integrations.project_id, integrations.id, integrations.type
from integrations
on conflict (project_id, type) do nothing;

alter table project_mcp_links enable row level security;

drop policy if exists project_mcp_links_select_all_authenticated on project_mcp_links;
create policy "project_mcp_links_select_all_authenticated"
  on project_mcp_links for select
  using (auth.role() = 'authenticated');

drop policy if exists project_mcp_links_write_all_authenticated on project_mcp_links;
create policy "project_mcp_links_write_all_authenticated"
  on project_mcp_links for all
  using (auth.role() = 'authenticated')
  with check (auth.role() = 'authenticated');
