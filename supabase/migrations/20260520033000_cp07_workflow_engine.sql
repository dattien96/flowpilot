-- CP-07: Canonical Workflow Engine Tables & Seed Data

-- The MVP skeleton already created legacy workflow runtime tables using TEXT ids:
--   workflow_runs
--   workflow_steps
-- We rename those legacy tables out of the way before creating the canonical
-- UUID-based workflow engine tables used by CP-07.
do $$
declare
  workflow_runs_id_type text;
  workflow_steps_id_type text;
begin
  select pg_catalog.format_type(a.atttypid, a.atttypmod)
  into workflow_runs_id_type
  from pg_attribute a
  join pg_class c on c.oid = a.attrelid
  join pg_namespace n on n.oid = c.relnamespace
  where n.nspname = 'public'
    and c.relname = 'workflow_runs'
    and a.attname = 'id'
    and not a.attisdropped;

  if workflow_runs_id_type = 'text' then
    alter table workflow_runs rename to legacy_workflow_runs;
  end if;

  select pg_catalog.format_type(a.atttypid, a.atttypmod)
  into workflow_steps_id_type
  from pg_attribute a
  join pg_class c on c.oid = a.attrelid
  join pg_namespace n on n.oid = c.relnamespace
  where n.nspname = 'public'
    and c.relname = 'workflow_steps'
    and a.attname = 'id'
    and not a.attisdropped;

  if workflow_steps_id_type = 'text' then
    alter table workflow_steps rename to legacy_workflow_steps;
  end if;
end $$;

create table if not exists step_definitions (
  step_type text primary key,
  name text not null,
  description text not null,
  required_mcps jsonb default '[]'::jsonb,
  required_skills jsonb default '[]'::jsonb,
  agent_type text not null default 'standard' check (agent_type in ('standard', 'autonomous'))
);

create table if not exists workflows (
  id uuid primary key default gen_random_uuid(),
  project_id uuid references projects(id) on delete cascade,
  name text not null,
  description text not null,
  is_template boolean not null default false,
  provider_override text,
  model_override text,
  created_by text not null default 'supabase-admin',
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

create table if not exists workflow_steps (
  id uuid primary key default gen_random_uuid(),
  workflow_id uuid not null references workflows(id) on delete cascade,
  step_type text not null references step_definitions(step_type),
  order_index int not null,
  is_enabled boolean not null default true,
  provider_override text,
  model_override text,
  requires_approval boolean not null default true,
  created_at timestamptz default now(),
  updated_at timestamptz default now(),
  unique (workflow_id, order_index)
);

create table if not exists workflow_prompt_cache (
  id uuid primary key default gen_random_uuid(),
  workflow_id uuid not null references workflows(id) on delete cascade,
  config_hash text not null unique,
  file_path text not null,
  step_type text not null references step_definitions(step_type),
  provider text not null check (provider in ('claude', 'codex', 'gemini')),
  is_valid boolean not null default true,
  created_at timestamptz default now(),
  invalidated_at timestamptz
);

create table if not exists workflow_runs (
  id uuid primary key default gen_random_uuid(),
  workflow_id uuid not null references workflows(id) on delete cascade,
  project_id uuid not null references projects(id) on delete cascade,
  status text not null default 'PENDING' check (status in ('PENDING', 'RUNNING', 'DONE', 'FAILED', 'CANCELED')),
  provider text,
  model text,
  yolo_mode boolean not null default false,
  started_by text not null default 'supabase-admin',
  started_at timestamptz default now(),
  finished_at timestamptz,
  error_message text
);

create table if not exists workflow_run_steps (
  id uuid primary key default gen_random_uuid(),
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  workflow_step_id uuid references workflow_steps(id) on delete set null,
  execution_order_index int not null default 0,
  step_type text not null references step_definitions(step_type),
  status text not null default 'PENDING' check (status in ('PENDING', 'RUNNING', 'WAITING_USER_APPROVAL', 'DONE', 'FAILED', 'SKIPPED')),
  artifact_id uuid, -- point to artifacts table (CP-06)
  prompt_cache_id uuid references workflow_prompt_cache(id) on delete set null,
  rejection_note text,
  retry_count int not null default 0,
  started_at timestamptz,
  finished_at timestamptz,
  error_message text
);

create table if not exists workflow_run_logs (
  id uuid primary key default gen_random_uuid(),
  workflow_run_step_id uuid not null references workflow_run_steps(id) on delete cascade,
  log_level text not null check (log_level in ('info', 'warn', 'error', 'debug')),
  message text not null,
  created_at timestamptz default now()
);

-- Indexes for fast querying
create index if not exists workflow_steps_workflow_idx on workflow_steps(workflow_id);
create index if not exists workflow_prompt_cache_hash_idx on workflow_prompt_cache(config_hash);
create index if not exists workflow_runs_project_idx on workflow_runs(project_id);
create index if not exists workflow_run_steps_run_idx on workflow_run_steps(workflow_run_id);
create index if not exists workflow_run_steps_run_order_idx on workflow_run_steps(workflow_run_id, execution_order_index);
create index if not exists workflow_run_logs_step_idx on workflow_run_logs(workflow_run_step_id);

-- Enable RLS
alter table step_definitions enable row level security;
alter table workflows enable row level security;
alter table workflow_steps enable row level security;
alter table workflow_prompt_cache enable row level security;
alter table workflow_runs enable row level security;
alter table workflow_run_steps enable row level security;
alter table workflow_run_logs enable row level security;

-- RLS Policies
create policy "step_definitions_select_authenticated"
  on step_definitions for select to authenticated using (true);
create policy "step_definitions_write_authenticated"
  on step_definitions for all to authenticated using (true) with check (true);

create policy "authenticated read workflows"
  on workflows for select to authenticated using (
    project_id is null
    or exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = workflows.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );
create policy "authenticated write private workflows"
  on workflows for all to authenticated using (
    project_id is not null
    and exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = workflows.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  ) with check (
    project_id is not null
    and exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = workflows.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

create policy "authenticated read workflow_steps"
  on workflow_steps for select to authenticated using (
    exists (
      select 1
      from workflows w
      left join project_teams pt on pt.project_id = w.project_id
      left join team_members tm on tm.team_id = pt.team_id
      where w.id = workflow_steps.workflow_id
        and (
          w.project_id is null
          or lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
        )
    )
  );
create policy "authenticated write private workflow_steps"
  on workflow_steps for all to authenticated using (
    exists (
      select 1
      from workflows w
      join project_teams pt on pt.project_id = w.project_id
      join team_members tm on tm.team_id = pt.team_id
      where w.id = workflow_steps.workflow_id
        and w.project_id is not null
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  ) with check (
    exists (
      select 1
      from workflows w
      join project_teams pt on pt.project_id = w.project_id
      join team_members tm on tm.team_id = pt.team_id
      where w.id = workflow_steps.workflow_id
        and w.project_id is not null
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

create policy "authenticated read workflow_prompt_cache"
  on workflow_prompt_cache for select to authenticated using (
    exists (
      select 1
      from workflows w
      left join project_teams pt on pt.project_id = w.project_id
      left join team_members tm on tm.team_id = pt.team_id
      where w.id = workflow_prompt_cache.workflow_id
        and (
          w.project_id is null
          or lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
        )
    )
  );
create policy "authenticated write workflow_prompt_cache"
  on workflow_prompt_cache for all to authenticated using (
    exists (
      select 1
      from workflows w
      left join project_teams pt on pt.project_id = w.project_id
      left join team_members tm on tm.team_id = pt.team_id
      where w.id = workflow_prompt_cache.workflow_id
        and (
          w.project_id is null
          or lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
        )
    )
  ) with check (
    exists (
      select 1
      from workflows w
      left join project_teams pt on pt.project_id = w.project_id
      left join team_members tm on tm.team_id = pt.team_id
      where w.id = workflow_prompt_cache.workflow_id
        and (
          w.project_id is null
          or lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
        )
    )
  );

create policy "authenticated read workflow_runs"
  on workflow_runs for select to authenticated using (
    exists (
      select 1
      from project_teams pt
      join team_members tm on tm.team_id = pt.team_id
      where pt.project_id = workflow_runs.project_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

create policy "authenticated read workflow_run_steps"
  on workflow_run_steps for select to authenticated using (
    exists (
      select 1
      from workflow_runs wr
      join project_teams pt on pt.project_id = wr.project_id
      join team_members tm on tm.team_id = pt.team_id
      where wr.id = workflow_run_steps.workflow_run_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

create policy "authenticated read workflow_run_logs"
  on workflow_run_logs for select to authenticated using (
    exists (
      select 1
      from workflow_run_steps wrs
      join workflow_runs wr on wr.id = wrs.workflow_run_id
      join project_teams pt on pt.project_id = wr.project_id
      join team_members tm on tm.team_id = pt.team_id
      where wrs.id = workflow_run_logs.workflow_run_step_id
        and lower(coalesce(tm.email, '')) = lower(coalesce(auth.jwt()->>'email', ''))
    )
  );

-- Seed static step definitions
insert into step_definitions (step_type, name, description, required_mcps, required_skills, agent_type) values
  ('business_idea', 'Business Idea', 'Capture and refine a raw business idea', '[]', '["business_analyst_skill"]', 'standard'),
  ('feature_intake', 'Feature Intake', 'Read and structure feature requirements from Jira', '["jira"]', '["feature_intake_skill"]', 'standard'),
  ('business_summary', 'Business Summary', 'Summarize business requirements into a PRD', '[]', '["business_summary_skill"]', 'standard'),
  ('product_spec', 'Product Spec', 'Generate a detailed product specification', '[]', '["product_spec_skill"]', 'standard'),
  ('tech_spec', 'Tech Spec', 'Generate a technical specification', '[]', '["tech_spec_skill"]', 'standard'),
  ('make_plan_coding', 'Make Plan Coding', 'Create a step-by-step coding plan', '[]', '["make_plan_coding_skill"]', 'standard'),
  ('create_architecture', 'Create Architecture', 'Design system architecture and components', '[]', '["create_architecture_skill"]', 'standard'),
  ('tdd', 'TDD', 'Create unit test signatures matching business requirements', '[]', '["tdd_skill"]', 'standard'),
  ('task_breakdown', 'Task Breakdown', 'Break coding plan into developer tasks with master schedule', '[]', '["task_breakdown_skill"]', 'standard'),
  ('code_review_loop', 'Code/Review Loop', 'Autonomously code, test, and review until passing', '[]', '["coding_skill", "review_skill"]', 'autonomous'),
  ('release_readiness', 'Release Readiness', 'Verify release criteria are met', '[]', '["release_readiness_skill"]', 'standard'),
  ('issue_analysis', 'Issue Analysis', 'Analyze crash reports and issues for root cause', '["firebase", "jira"]', '["issue_analysis_skill"]', 'standard'),
  ('analytics_review', 'Analytics Review', 'Review usage data and user behavior insights', '["firebase"]', '["analytics_review_skill"]', 'standard'),
  ('project_analysis', 'Project Analysis', 'Analyze project health and process bottlenecks', '["google_drive"]', '["project_analysis_skill"]', 'standard'),
  ('telegram_notification', 'Telegram Notification', 'Send workflow status to Telegram chat', '["telegram"]', '["notification_skill"]', 'standard'),
  ('code_traceability', 'Code Traceability', 'Trace code/bug back to original Jira ticket', '["jira"]', '["code_traceability_skill"]', 'standard'),
  ('onboarding_walkthrough', 'Onboarding Walkthrough', 'Generate product/codebase summary for new members', '["google_drive"]', '["onboarding_skill"]', 'standard')
on conflict (step_type) do update set
  name = excluded.name,
  description = excluded.description,
  required_mcps = excluded.required_mcps,
  required_skills = excluded.required_skills,
  agent_type = excluded.agent_type;

-- Seed built-in workflow templates (is_template = true, scoped to a dummy or seed project)
-- We use project_meal_suggestion as the anchor for these built-in template seeds.
do $$
declare
  t_bug_fix uuid;
  t_pre_def uuid;
  t_trace uuid;
  t_onboard uuid;
  t_e2e uuid;
  t_fast uuid;
  t_task uuid;
  t_rca uuid;
  t_analytics uuid;
  t_process uuid;
begin
  -- 1. Bug Fix Flow
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Bug Fix Flow', 'Traceability -> Issue Analysis -> Tech Spec -> Plan -> Code/Review -> Release', true)
  returning id into t_bug_fix;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_bug_fix, 'code_traceability', 0, false),
    (t_bug_fix, 'issue_analysis', 1, false),
    (t_bug_fix, 'tech_spec', 2, true),
    (t_bug_fix, 'make_plan_coding', 3, true),
    (t_bug_fix, 'code_review_loop', 4, true),
    (t_bug_fix, 'release_readiness', 5, true);

  -- 2. Pre-defined Feature
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Pre-defined Feature', 'Tech Spec -> Plan -> Architecture -> TDD -> Code/Review -> Release -> Notify', true)
  returning id into t_pre_def;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_pre_def, 'tech_spec', 0, true),
    (t_pre_def, 'make_plan_coding', 1, true),
    (t_pre_def, 'create_architecture', 2, true),
    (t_pre_def, 'tdd', 3, true),
    (t_pre_def, 'code_review_loop', 4, true),
    (t_pre_def, 'release_readiness', 5, true),
    (t_pre_def, 'telegram_notification', 6, false);

  -- 3. Bug Traceability
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Bug Traceability', 'Code Traceability single step', true)
  returning id into t_trace;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_trace, 'code_traceability', 0, false);

  -- 4. Onboarding
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Onboarding', 'Onboarding Walkthrough single step', true)
  returning id into t_onboard;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_onboard, 'onboarding_walkthrough', 0, false);

  -- 5. Full End-to-End
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Full End-to-End', 'Complete Solo Dev pipeline from Business Idea to Notification', true)
  returning id into t_e2e;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_e2e, 'business_idea', 0, true),
    (t_e2e, 'feature_intake', 1, false),
    (t_e2e, 'business_summary', 2, true),
    (t_e2e, 'product_spec', 3, true),
    (t_e2e, 'tech_spec', 4, true),
    (t_e2e, 'make_plan_coding', 5, true),
    (t_e2e, 'create_architecture', 6, true),
    (t_e2e, 'tdd', 7, true),
    (t_e2e, 'code_review_loop', 8, true),
    (t_e2e, 'release_readiness', 9, true),
    (t_e2e, 'telegram_notification', 10, false);

  -- 6. Fast-Track Business
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Fast-Track Business', 'Product Spec -> Tech Spec -> Plan -> Arch -> TDD -> Code/Review -> Release', true)
  returning id into t_fast;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_fast, 'product_spec', 0, true),
    (t_fast, 'tech_spec', 1, true),
    (t_fast, 'make_plan_coding', 2, true),
    (t_fast, 'create_architecture', 3, true),
    (t_fast, 'tdd', 4, true),
    (t_fast, 'code_review_loop', 5, true),
    (t_fast, 'release_readiness', 6, true);

  -- 7. Task Breakdown
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Task Breakdown', 'Tech Spec -> Plan -> Task Breakdown', true)
  returning id into t_task;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_task, 'tech_spec', 0, true),
    (t_task, 'make_plan_coding', 1, true),
    (t_task, 'task_breakdown', 2, true);

  -- 8. Root Cause Analysis
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Root Cause Analysis', 'Traceability -> Issue Analysis -> Task Breakdown -> Notify', true)
  returning id into t_rca;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_rca, 'code_traceability', 0, false),
    (t_rca, 'issue_analysis', 1, false),
    (t_rca, 'task_breakdown', 2, true),
    (t_rca, 'telegram_notification', 3, false);

  -- 9. Analytics & Usage
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Analytics & Usage', 'Analytics Review single step', true)
  returning id into t_analytics;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_analytics, 'analytics_review', 0, false);

  -- 10. Product Process & Analysis
  insert into workflows (id, project_id, name, description, is_template)
  values (gen_random_uuid(), null, 'Product Process & Analysis', 'Business Idea -> Feature Intake -> Business Summary -> Product Spec -> Project Analysis -> Analytics Review', true)
  returning id into t_process;

  insert into workflow_steps (workflow_id, step_type, order_index, requires_approval) values
    (t_process, 'business_idea', 0, true),
    (t_process, 'feature_intake', 1, false),
    (t_process, 'business_summary', 2, true),
    (t_process, 'product_spec', 3, true),
    (t_process, 'project_analysis', 4, true),
    (t_process, 'analytics_review', 5, false);

end $$;
