create table if not exists profiles (
  id text primary key,
  email text,
  role text default 'owner',
  created_at timestamptz default now()
);

create table if not exists projects (
  id text primary key,
  name text not null,
  description text not null,
  platform text not null,
  repository_url text not null,
  created_by text not null,
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

create table if not exists features (
  id text primary key,
  project_id text not null references projects(id) on delete cascade,
  title text not null,
  business_goal text not null,
  user_problem text not null,
  expected_flow text not null,
  acceptance_criteria text not null,
  priority text not null,
  status text not null,
  owner_id text not null,
  created_at timestamptz default now(),
  updated_at timestamptz default now()
);

create table if not exists context_sources (
  id text primary key,
  project_id text not null references projects(id) on delete cascade,
  feature_id text references features(id) on delete cascade,
  type text not null,
  title text not null,
  raw_content text not null,
  summarized_content text,
  archived_at timestamptz,
  created_by text not null,
  created_at timestamptz default now()
);

create table if not exists workflow_definitions (
  id text primary key,
  name text not null,
  description text not null,
  version int not null,
  status text not null,
  definition jsonb not null default '{}'::jsonb,
  created_at timestamptz default now()
);

create table if not exists workflow_runs (
  id text primary key,
  workflow_definition_id text not null references workflow_definitions(id),
  project_id text not null references projects(id),
  feature_id text not null references features(id),
  status text not null,
  current_step_key text,
  selected_context_source_ids jsonb not null default '[]'::jsonb,
  started_by text not null,
  started_at timestamptz default now(),
  completed_at timestamptz,
  error_summary text
);

create table if not exists workflow_steps (
  id text primary key,
  workflow_run_id text not null references workflow_runs(id) on delete cascade,
  step_key text not null,
  step_name text not null,
  step_type text not null,
  status text not null,
  sequence_index int not null,
  output_id text,
  started_at timestamptz,
  completed_at timestamptz,
  error_message text
);

create table if not exists ai_outputs (
  id text primary key,
  workflow_run_id text not null references workflow_runs(id) on delete cascade,
  workflow_step_id text not null references workflow_steps(id) on delete cascade,
  project_id text not null references projects(id),
  feature_id text not null references features(id),
  output_type text not null,
  version int not null default 1,
  title text not null,
  content_markdown text not null,
  is_approved boolean not null default false,
  created_at timestamptz default now()
);

create table if not exists approvals (
  id text primary key,
  workflow_run_id text not null references workflow_runs(id) on delete cascade,
  workflow_step_id text not null references workflow_steps(id) on delete cascade,
  ai_output_id text references ai_outputs(id) on delete set null,
  status text not null,
  reviewer_id text,
  comment text,
  decided_at timestamptz,
  created_at timestamptz default now()
);

create table if not exists approval_decisions (
  id text primary key,
  approval_id text not null references approvals(id) on delete cascade,
  workflow_run_id text not null references workflow_runs(id) on delete cascade,
  workflow_step_id text not null references workflow_steps(id) on delete cascade,
  ai_output_id text references ai_outputs(id) on delete set null,
  decision text not null,
  reviewer_id text,
  comment text,
  created_at timestamptz default now()
);

create table if not exists ai_call_logs (
  id text primary key,
  workflow_run_id text not null references workflow_runs(id) on delete cascade,
  workflow_step_id text not null references workflow_steps(id) on delete cascade,
  provider text not null,
  model text not null,
  input_tokens int not null default 0,
  output_tokens int not null default 0,
  cost_estimate numeric(10, 4) not null default 0,
  latency_ms int not null default 0,
  status text not null,
  created_at timestamptz default now()
);

create index if not exists features_project_id_idx on features(project_id);
create index if not exists context_sources_project_id_idx on context_sources(project_id);
create index if not exists context_sources_feature_id_idx on context_sources(feature_id);
create index if not exists workflow_runs_feature_id_idx on workflow_runs(feature_id);
create index if not exists workflow_steps_run_id_idx on workflow_steps(workflow_run_id);
create index if not exists ai_outputs_run_id_idx on ai_outputs(workflow_run_id);
create index if not exists approvals_status_idx on approvals(status);
create index if not exists approval_decisions_approval_id_idx on approval_decisions(approval_id);
create index if not exists approval_decisions_run_id_idx on approval_decisions(workflow_run_id);
create index if not exists approval_decisions_output_id_idx on approval_decisions(ai_output_id);
create index if not exists ai_call_logs_run_id_idx on ai_call_logs(workflow_run_id);

alter table profiles enable row level security;
alter table projects enable row level security;
alter table features enable row level security;
alter table context_sources enable row level security;
alter table workflow_definitions enable row level security;
alter table workflow_runs enable row level security;
alter table workflow_steps enable row level security;
alter table ai_outputs enable row level security;
alter table approvals enable row level security;
alter table approval_decisions enable row level security;
alter table ai_call_logs enable row level security;

create policy "authenticated read profiles"
  on profiles for select to authenticated using (true);
create policy "authenticated read projects"
  on projects for select to authenticated using (true);
create policy "authenticated write projects"
  on projects for all to authenticated using (true) with check (true);
create policy "authenticated read features"
  on features for select to authenticated using (true);
create policy "authenticated write features"
  on features for all to authenticated using (true) with check (true);
create policy "authenticated read context"
  on context_sources for select to authenticated using (true);
create policy "authenticated write context"
  on context_sources for all to authenticated using (true) with check (true);
create policy "authenticated read definitions"
  on workflow_definitions for select to authenticated using (true);
create policy "authenticated read runs"
  on workflow_runs for select to authenticated using (true);
create policy "authenticated write runs"
  on workflow_runs for all to authenticated using (true) with check (true);
create policy "authenticated read steps"
  on workflow_steps for select to authenticated using (true);
create policy "authenticated write steps"
  on workflow_steps for all to authenticated using (true) with check (true);
create policy "authenticated read outputs"
  on ai_outputs for select to authenticated using (true);
create policy "authenticated write outputs"
  on ai_outputs for all to authenticated using (true) with check (true);
create policy "authenticated read approvals"
  on approvals for select to authenticated using (true);
create policy "authenticated write approvals"
  on approvals for all to authenticated using (true) with check (true);
create policy "authenticated read approval decisions"
  on approval_decisions for select to authenticated using (true);
create policy "authenticated write approval decisions"
  on approval_decisions for all to authenticated using (true) with check (true);
create policy "authenticated read logs"
  on ai_call_logs for select to authenticated using (true);
create policy "authenticated write logs"
  on ai_call_logs for all to authenticated using (true) with check (true);

insert into projects (id, name, description, platform, repository_url, created_by)
values
  (
    'project_meal_suggestion',
    'Meal Suggestion Android App',
    'Android product used to validate feature planning and widget flows.',
    'android',
    'https://github.com/example/meal-suggestion',
    'seed'
  ),
  (
    'project_flowpilot_admin',
    'FlowPilot Admin Web',
    'Internal admin surface for workflow orchestration, approvals, and engineering reports.',
    'web',
    'https://github.com/example/flowpilot-admin',
    'seed'
  )
on conflict (id) do nothing;

insert into features (
  id,
  project_id,
  title,
  business_goal,
  user_problem,
  expected_flow,
  acceptance_criteria,
  priority,
  status,
  owner_id
)
values
  (
    'feature_widget_refresh',
    'project_meal_suggestion',
    'Home Widget Meal Card Refresh',
    'Increase home screen engagement with glanceable meal content.',
    'Users want a fresh meal suggestion without opening the app.',
    'Widget displays meal image, title, and type. Refresh swaps the content in place.',
    'Refresh updates content, loading is graceful, and analytics capture interaction.',
    'high',
    'active',
    'seed'
  ),
  (
    'feature_admin_approval_center',
    'project_flowpilot_admin',
    'Approval Center Queue',
    'Make pending workflow approvals visible in one place.',
    'The owner loses track of workflow pauses spread across multiple features and runs.',
    'Approval queue highlights which output is blocked, why it matters, and what happens after approval.',
    'Queue can deep link to the related run and clearly show approval urgency.',
    'high',
    'active',
    'seed'
  )
on conflict (id) do nothing;

insert into context_sources (
  id,
  project_id,
  feature_id,
  type,
  title,
  raw_content,
  summarized_content,
  created_by
)
values
  (
    'context_widget_goals',
    'project_meal_suggestion',
    'feature_widget_refresh',
    'manual_text',
    'Business and UX notes',
    'Widget needs common Android UX, a visible refresh control, and a low-friction loading state.',
    null,
    'seed'
  ),
  (
    'context_admin_review',
    'project_flowpilot_admin',
    'feature_admin_approval_center',
    'manual_text',
    'Approval center UX notes',
    'Queue should prioritize waiting approvals, surface step names, and keep decision actions close to output context.',
    null,
    'seed'
  )
on conflict (id) do nothing;

insert into workflow_definitions (id, name, description, version, status, definition)
values (
  'workflow_feature_to_android_tech_spec',
  'feature_to_android_tech_spec',
  'Convert a feature intake into business summary, product spec, Android tech spec, task breakdown, test plan, and risk report.',
  1,
  'active',
  '{
    "steps": [
      { "key": "collect_context", "name": "Collect Context", "type": "tool" },
      { "key": "generate_business_summary", "name": "Generate Business Summary", "type": "ai_mock", "outputType": "business_summary" },
      { "key": "approval_business_summary", "name": "Approve Business Summary", "type": "approval" },
      { "key": "generate_product_spec", "name": "Generate Product Spec", "type": "ai_mock", "outputType": "product_spec" },
      { "key": "approval_product_spec", "name": "Approve Product Spec", "type": "approval" },
      { "key": "generate_android_tech_spec", "name": "Generate Android Tech Spec", "type": "ai_mock", "outputType": "android_tech_spec" },
      { "key": "approval_android_tech_spec", "name": "Approve Android Tech Spec", "type": "approval" },
      { "key": "generate_task_breakdown", "name": "Generate Task Breakdown", "type": "ai_mock", "outputType": "task_breakdown" },
      { "key": "generate_test_plan", "name": "Generate Test Plan", "type": "ai_mock", "outputType": "test_plan" },
      { "key": "generate_risk_report", "name": "Generate Risk Report", "type": "ai_mock", "outputType": "risk_report" }
    ]
  }'::jsonb
)
on conflict (id) do nothing;
