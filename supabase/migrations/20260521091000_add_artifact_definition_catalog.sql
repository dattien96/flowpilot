create table if not exists artifact_definitions (
  key text primary key,
  name text not null,
  description text not null default '',
  local_path_template text not null,
  remote_path_template text not null default '',
  default_file_name text not null default '',
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

create table if not exists step_input_artifact_definitions (
  step_type text not null references step_definitions(step_type) on delete cascade,
  artifact_definition_key text not null references artifact_definitions(key) on delete cascade,
  order_index int not null default 0,
  created_at timestamptz default now(),
  primary key (step_type, artifact_definition_key)
);

create table if not exists step_output_artifact_definitions (
  step_type text not null references step_definitions(step_type) on delete cascade,
  artifact_definition_key text not null references artifact_definitions(key) on delete cascade,
  order_index int not null default 0,
  created_at timestamptz default now(),
  primary key (step_type, artifact_definition_key)
);

create table if not exists artifact_runs (
  id uuid primary key default gen_random_uuid(),
  artifact_definition_key text not null references artifact_definitions(key) on delete restrict,
  project_id uuid not null references projects(id) on delete cascade,
  workflow_id uuid not null references workflows(id) on delete cascade,
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  workflow_run_step_id uuid references workflow_run_steps(id) on delete set null,
  title text not null,
  local_path text not null,
  remote_path text not null default '',
  remote_url text not null default '',
  sync_status text not null default 'local_only' check (sync_status in ('local_only', 'queued', 'syncing', 'synced', 'failed')),
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

alter table workflow_run_steps
  add column if not exists artifact_run_id uuid references artifact_runs(id) on delete set null;

create index if not exists step_input_artifact_definitions_step_idx
  on step_input_artifact_definitions(step_type);
create index if not exists step_output_artifact_definitions_step_idx
  on step_output_artifact_definitions(step_type);
create index if not exists artifact_runs_project_idx
  on artifact_runs(project_id);
create index if not exists artifact_runs_workflow_idx
  on artifact_runs(workflow_id);
create index if not exists artifact_runs_run_idx
  on artifact_runs(workflow_run_id);
create index if not exists artifact_runs_step_idx
  on artifact_runs(workflow_run_step_id);

alter table artifact_definitions enable row level security;
alter table step_input_artifact_definitions enable row level security;
alter table step_output_artifact_definitions enable row level security;
alter table artifact_runs enable row level security;

create policy "authenticated read artifact_definitions"
  on artifact_definitions for select to authenticated using (true);
create policy "authenticated write artifact_definitions"
  on artifact_definitions for all to authenticated using (true) with check (true);

create policy "authenticated read step_input_artifact_definitions"
  on step_input_artifact_definitions for select to authenticated using (true);
create policy "authenticated write step_input_artifact_definitions"
  on step_input_artifact_definitions for all to authenticated using (true) with check (true);

create policy "authenticated read step_output_artifact_definitions"
  on step_output_artifact_definitions for select to authenticated using (true);
create policy "authenticated write step_output_artifact_definitions"
  on step_output_artifact_definitions for all to authenticated using (true) with check (true);

create policy "authenticated read artifact_runs"
  on artifact_runs for select to authenticated using (
    exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = artifact_runs.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

do $$
begin
  if exists (
    select 1
    from information_schema.columns
    where table_name = 'step_definitions'
      and column_name = 'input_artifact_definitions'
  ) then
    insert into step_input_artifact_definitions (step_type, artifact_definition_key, order_index)
    select
      sd.step_type,
      value,
      ordinality - 1
    from step_definitions sd,
         jsonb_array_elements_text(coalesce(sd.input_artifact_definitions, '[]'::jsonb)) with ordinality as input(value, ordinality)
    on conflict (step_type, artifact_definition_key) do update
      set order_index = excluded.order_index;
  end if;

  if exists (
    select 1
    from information_schema.columns
    where table_name = 'step_definitions'
      and column_name = 'output_artifact_definitions'
  ) then
    insert into step_output_artifact_definitions (step_type, artifact_definition_key, order_index)
    select
      sd.step_type,
      value,
      ordinality - 1
    from step_definitions sd,
         jsonb_array_elements_text(coalesce(sd.output_artifact_definitions, '[]'::jsonb)) with ordinality as output(value, ordinality)
    on conflict (step_type, artifact_definition_key) do update
      set order_index = excluded.order_index;
  end if;
end $$;

insert into artifact_definitions (
  key,
  name,
  description,
  local_path_template,
  remote_path_template,
  default_file_name,
  created_at,
  updated_at
) values
  ('business_idea_artifact', 'Business Idea', 'Business idea artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/BusinessIdea.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/BusinessIdea.md', 'BusinessIdea.md', now(), now()),
  ('feature_intake_artifact', 'Feature Intake', 'Feature intake artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/FeatureIntake.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/FeatureIntake.md', 'FeatureIntake.md', now(), now()),
  ('business_summary_artifact', 'Business Summary', 'Business summary artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/BusinessSummary.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/BusinessSummary.md', 'BusinessSummary.md', now(), now()),
  ('product_spec_artifact', 'Product Spec', 'Product spec artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/ProductSpec.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/ProductSpec.md', 'ProductSpec.md', now(), now()),
  ('tech_spec_artifact', 'Tech Spec', 'Tech spec artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/TechSpec.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/TechSpec.md', 'TechSpec.md', now(), now()),
  ('coding_plan_artifact', 'Coding Plan', 'Coding plan artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/CodingPlan.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/CodingPlan.md', 'CodingPlan.md', now(), now()),
  ('architecture_artifact', 'Architecture', 'Architecture artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/ArchitecturePlan.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/ArchitecturePlan.md', 'ArchitecturePlan.md', now(), now()),
  ('tdd_plan_artifact', 'TDD Plan', 'TDD plan artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/TddPlan.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/TddPlan.md', 'TddPlan.md', now(), now()),
  ('task_breakdown_artifact', 'Task Breakdown', 'Task breakdown artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/TaskBreakdown.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/TaskBreakdown.md', 'TaskBreakdown.md', now(), now()),
  ('code_review_summary_artifact', 'Code Review Summary', 'Code review summary artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/CodeReviewSummary.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/CodeReviewSummary.md', 'CodeReviewSummary.md', now(), now()),
  ('release_readiness_artifact', 'Release Readiness', 'Release readiness artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/ReleaseReadiness.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/ReleaseReadiness.md', 'ReleaseReadiness.md', now(), now()),
  ('root_cause_analysis_artifact', 'Root Cause Analysis', 'Root cause analysis artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/RootCauseAnalysis.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/RootCauseAnalysis.md', 'RootCauseAnalysis.md', now(), now()),
  ('usage_analytics_artifact', 'Usage Analytics', 'Usage analytics artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/UsageAnalytics.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/UsageAnalytics.md', 'UsageAnalytics.md', now(), now()),
  ('project_analysis_artifact', 'Project Analysis', 'Project analysis artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/ProjectAnalysis.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/ProjectAnalysis.md', 'ProjectAnalysis.md', now(), now()),
  ('code_traceability_artifact', 'Code Traceability', 'Code traceability artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/CodeTraceability.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/CodeTraceability.md', 'CodeTraceability.md', now(), now()),
  ('onboarding_walkthrough_artifact', 'Onboarding Walkthrough', 'Onboarding walkthrough artifact', '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/OnboardingWalkthrough.md', 'artifacts/{projectId}/{workflowRunId}/{stepType}/OnboardingWalkthrough.md', 'OnboardingWalkthrough.md', now(), now())
on conflict (key) do update
  set name = excluded.name,
      description = excluded.description,
      local_path_template = excluded.local_path_template,
      remote_path_template = excluded.remote_path_template,
      default_file_name = excluded.default_file_name,
      updated_at = now();

do $$
begin
  if exists (
    select 1
    from information_schema.columns
    where table_name = 'step_definitions'
      and column_name = 'input_artifact_definitions'
  ) then
    alter table step_definitions drop column if exists input_artifact_definitions;
  end if;

  if exists (
    select 1
    from information_schema.columns
    where table_name = 'step_definitions'
      and column_name = 'output_artifact_definitions'
  ) then
    alter table step_definitions drop column if exists output_artifact_definitions;
  end if;
end $$;
