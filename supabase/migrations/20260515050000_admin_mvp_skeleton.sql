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
