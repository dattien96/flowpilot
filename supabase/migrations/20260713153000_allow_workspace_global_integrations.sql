alter table integrations
  alter column project_id drop not null;

drop index if exists integrations_project_type_idx;
create index if not exists integrations_project_type_idx on integrations(project_id, type);
