create table if not exists ai_prompt_templates (
  id uuid primary key default gen_random_uuid(),
  project_id uuid references projects(id) on delete cascade,
  step_type text not null references step_definitions(step_type) on delete cascade,
  name text not null,
  description text not null default '',
  input_schema jsonb not null default '{}'::jsonb,
  output_schema jsonb not null default '{}'::jsonb,
  template_content text not null,
  provider_preference text,
  model_preference text,
  version int not null default 1,
  status text not null default 'active' check (status in ('active', 'archived')),
  created_by uuid references auth.users(id) on delete set null,
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

create unique index if not exists ai_prompt_templates_scope_step_version_uniq
  on ai_prompt_templates (
    coalesce(project_id, '00000000-0000-0000-0000-000000000000'::uuid),
    step_type,
    version
  );

create index if not exists ai_prompt_templates_step_type_idx
  on ai_prompt_templates(step_type);

create index if not exists ai_prompt_templates_project_idx
  on ai_prompt_templates(project_id);

create table if not exists ai_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_type text not null,
  input_payload jsonb not null default '{}'::jsonb,
  output_payload jsonb,
  model_name text not null,
  triggered_by uuid references auth.users(id) on delete set null,
  status text not null default 'running' check (status in ('running', 'success', 'failed')),
  error_message text,
  tokens_input int,
  tokens_output int,
  cost_usd numeric(10, 4),
  prompt_template_id uuid references ai_prompt_templates(id) on delete set null,
  workflow_run_id uuid references workflow_runs(id) on delete set null,
  workflow_run_step_id uuid references workflow_run_steps(id) on delete set null,
  completed_at timestamptz,
  created_at timestamptz default now()
);

create index if not exists ai_runs_project_idx
  on ai_runs(project_id);

create index if not exists ai_runs_status_idx
  on ai_runs(status);

create index if not exists ai_runs_created_at_idx
  on ai_runs(created_at desc);

create index if not exists ai_runs_workflow_run_idx
  on ai_runs(workflow_run_id);

create index if not exists ai_runs_workflow_step_idx
  on ai_runs(workflow_run_step_id);

create table if not exists artifact_annotations (
  id uuid primary key default gen_random_uuid(),
  artifact_run_id uuid not null references artifact_runs(id) on delete cascade,
  user_id uuid not null references auth.users(id) on delete cascade,
  highlight_range text,
  note_text text not null,
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

create index if not exists artifact_annotations_artifact_run_idx
  on artifact_annotations(artifact_run_id);

create index if not exists artifact_annotations_user_idx
  on artifact_annotations(user_id);

alter table ai_prompt_templates enable row level security;
alter table ai_runs enable row level security;
alter table artifact_annotations enable row level security;

drop policy if exists "ai_prompt_templates_select_authenticated" on ai_prompt_templates;
create policy "ai_prompt_templates_select_authenticated"
  on ai_prompt_templates for select to authenticated
  using (
    project_id is null
    or exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_prompt_templates.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "ai_prompt_templates_insert_authenticated" on ai_prompt_templates;
create policy "ai_prompt_templates_insert_authenticated"
  on ai_prompt_templates for insert to authenticated
  with check (
    project_id is null
    or exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_prompt_templates.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "ai_prompt_templates_update_project_members" on ai_prompt_templates;
create policy "ai_prompt_templates_update_project_members"
  on ai_prompt_templates for update to authenticated
  using (
    project_id is not null
    and exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_prompt_templates.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  )
  with check (
    project_id is not null
    and exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_prompt_templates.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "ai_prompt_templates_delete_project_members" on ai_prompt_templates;
create policy "ai_prompt_templates_delete_project_members"
  on ai_prompt_templates for delete to authenticated
  using (
    project_id is not null
    and exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_prompt_templates.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "ai_runs_select_project_members" on ai_runs;
create policy "ai_runs_select_project_members"
  on ai_runs for select to authenticated
  using (
    exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_runs.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "ai_runs_insert_authenticated" on ai_runs;
create policy "ai_runs_insert_authenticated"
  on ai_runs for insert to authenticated
  with check (
    exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_runs.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "ai_runs_update_project_members" on ai_runs;
create policy "ai_runs_update_project_members"
  on ai_runs for update to authenticated
  using (
    exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_runs.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  )
  with check (
    exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = ai_runs.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "artifact_annotations_select" on artifact_annotations;
create policy "artifact_annotations_select"
  on artifact_annotations for select to authenticated
  using (
    exists (
      select 1
      from artifact_runs ar
      join project_teams pt on pt.project_id = ar.project_id
      join team_members tm on tm.team_id = pt.team_id
      where ar.id = artifact_annotations.artifact_run_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "artifact_annotations_insert" on artifact_annotations;
create policy "artifact_annotations_insert"
  on artifact_annotations for insert to authenticated
  with check (
    exists (
      select 1
      from artifact_runs ar
      join project_teams pt on pt.project_id = ar.project_id
      join team_members tm on tm.team_id = pt.team_id
      where ar.id = artifact_annotations.artifact_run_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "artifact_annotations_update" on artifact_annotations;
create policy "artifact_annotations_update"
  on artifact_annotations for update to authenticated
  using (
    exists (
      select 1
      from artifact_runs ar
      join project_teams pt on pt.project_id = ar.project_id
      join team_members tm on tm.team_id = pt.team_id
      where ar.id = artifact_annotations.artifact_run_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  )
  with check (
    exists (
      select 1
      from artifact_runs ar
      join project_teams pt on pt.project_id = ar.project_id
      join team_members tm on tm.team_id = pt.team_id
      where ar.id = artifact_annotations.artifact_run_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

drop policy if exists "artifact_annotations_delete" on artifact_annotations;
create policy "artifact_annotations_delete"
  on artifact_annotations for delete to authenticated
  using (
    exists (
      select 1
      from artifact_runs ar
      join project_teams pt on pt.project_id = ar.project_id
      join team_members tm on tm.team_id = pt.team_id
      where ar.id = artifact_annotations.artifact_run_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );
