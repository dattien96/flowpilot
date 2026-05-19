create extension if not exists pgcrypto;

-- Convert projects.id and dependent project_id foreign keys from TEXT to UUID.
-- Keep legacy text columns for one release so seeded slugs and rollback remain traceable.

alter table projects add column if not exists id_uuid uuid;
update projects
set id_uuid = gen_random_uuid()
where id_uuid is null;

alter table projects
  alter column id_uuid set not null,
  alter column id_uuid set default gen_random_uuid();

alter table features add column if not exists project_id_uuid uuid;
alter table context_sources add column if not exists project_id_uuid uuid;
alter table workflow_runs add column if not exists project_id_uuid uuid;
alter table ai_outputs add column if not exists project_id_uuid uuid;
alter table project_teams add column if not exists project_id_uuid uuid;

update features f
set project_id_uuid = p.id_uuid
from projects p
where f.project_id = p.id
  and f.project_id_uuid is null;

update context_sources cs
set project_id_uuid = p.id_uuid
from projects p
where cs.project_id = p.id
  and cs.project_id_uuid is null;

update workflow_runs wr
set project_id_uuid = p.id_uuid
from projects p
where wr.project_id = p.id
  and wr.project_id_uuid is null;

update ai_outputs ao
set project_id_uuid = p.id_uuid
from projects p
where ao.project_id = p.id
  and ao.project_id_uuid is null;

update project_teams pt
set project_id_uuid = p.id_uuid
from projects p
where pt.project_id = p.id
  and pt.project_id_uuid is null;

do $$
begin
  if exists (
    select 1 from features
    where project_id is not null and project_id_uuid is null
  ) then
    raise exception 'features.project_id contains rows that could not be mapped to projects.id_uuid';
  end if;

  if exists (
    select 1 from context_sources
    where project_id is not null and project_id_uuid is null
  ) then
    raise exception 'context_sources.project_id contains rows that could not be mapped to projects.id_uuid';
  end if;

  if exists (
    select 1 from workflow_runs
    where project_id is not null and project_id_uuid is null
  ) then
    raise exception 'workflow_runs.project_id contains rows that could not be mapped to projects.id_uuid';
  end if;

  if exists (
    select 1 from ai_outputs
    where project_id is not null and project_id_uuid is null
  ) then
    raise exception 'ai_outputs.project_id contains rows that could not be mapped to projects.id_uuid';
  end if;

  if exists (
    select 1 from project_teams
    where project_id is not null and project_id_uuid is null
  ) then
    raise exception 'project_teams.project_id contains rows that could not be mapped to projects.id_uuid';
  end if;
end $$;

alter table features drop constraint if exists features_project_id_fkey;
alter table context_sources drop constraint if exists context_sources_project_id_fkey;
alter table workflow_runs drop constraint if exists workflow_runs_project_id_fkey;
alter table ai_outputs drop constraint if exists ai_outputs_project_id_fkey;
alter table project_teams drop constraint if exists project_teams_project_id_fkey;
alter table project_teams drop constraint if exists project_teams_project_id_team_id_key;

drop index if exists features_project_id_idx;
drop index if exists context_sources_project_id_idx;

alter table projects drop constraint if exists projects_pkey;

alter table projects rename column id to legacy_id;
alter table projects rename column id_uuid to id;

alter table projects
  add constraint projects_pkey primary key (id),
  add constraint projects_legacy_id_key unique (legacy_id);

comment on column projects.legacy_id is
  'Legacy text project id retained temporarily after UUID baseline migration.';

alter table features rename column project_id to legacy_project_id;
alter table features rename column project_id_uuid to project_id;
alter table features alter column project_id set not null;
alter table features
  add constraint features_project_id_fkey
  foreign key (project_id) references projects(id) on delete cascade;

alter table context_sources rename column project_id to legacy_project_id;
alter table context_sources rename column project_id_uuid to project_id;
alter table context_sources alter column project_id set not null;
alter table context_sources
  add constraint context_sources_project_id_fkey
  foreign key (project_id) references projects(id) on delete cascade;

alter table workflow_runs rename column project_id to legacy_project_id;
alter table workflow_runs rename column project_id_uuid to project_id;
alter table workflow_runs alter column project_id set not null;
alter table workflow_runs
  add constraint workflow_runs_project_id_fkey
  foreign key (project_id) references projects(id);

alter table ai_outputs rename column project_id to legacy_project_id;
alter table ai_outputs rename column project_id_uuid to project_id;
alter table ai_outputs alter column project_id set not null;
alter table ai_outputs
  add constraint ai_outputs_project_id_fkey
  foreign key (project_id) references projects(id);

alter table project_teams rename column project_id to legacy_project_id;
alter table project_teams rename column project_id_uuid to project_id;
alter table project_teams alter column project_id set not null;
alter table project_teams
  add constraint project_teams_project_id_fkey
  foreign key (project_id) references projects(id) on delete cascade;
alter table project_teams
  add constraint project_teams_project_id_team_id_key unique (project_id, team_id);

create index if not exists features_project_id_idx on features(project_id);
create index if not exists context_sources_project_id_idx on context_sources(project_id);

comment on column features.legacy_project_id is
  'Legacy text project id retained temporarily after UUID baseline migration.';
comment on column context_sources.legacy_project_id is
  'Legacy text project id retained temporarily after UUID baseline migration.';
comment on column workflow_runs.legacy_project_id is
  'Legacy text project id retained temporarily after UUID baseline migration.';
comment on column ai_outputs.legacy_project_id is
  'Legacy text project id retained temporarily after UUID baseline migration.';
comment on column project_teams.legacy_project_id is
  'Legacy text project id retained temporarily after UUID baseline migration.';

-- Intentionally out of scope here:
--   * projects.created_by / owner_id
--   * features.owner_id
-- Those actor columns currently contain legacy text values like `seed`.
-- They need a separate auth.users mapping migration before they can become UUID FKs.
