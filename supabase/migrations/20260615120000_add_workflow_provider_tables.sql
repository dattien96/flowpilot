-- Migration: Add workflow_provider_* tables (app-server / controlled-mode provider runtime)
--
-- It's the durable pointer that links a FlowPilot run to the provider-side conversation, 
-- so a run can be resumed after a runner restart and audited. 
-- It does not store the chat history — the provider (Codex/Claude) owns that on disk; 
-- this table just stores the binding.

-- Backs the provider-runtime persistence referenced by the New-System refactor (03/04-02)
-- and the Claude adapter (07): normalized provider sessions, event telemetry, approvals,
-- and structured questions. These are provider-runtime telemetry tables; they do NOT
-- replace workflow_runs / workflow_run_steps (authoritative workflow state) or the older
-- workflow_run_sessions. Closes the "schema gap" noted in 05.

-- ---------------------------------------------------------------------------
-- Provider sessions: the (run, cwd, provider thread/session) mapping for resume.
-- For Codex: provider_session_id == provider_thread_id == Codex thread id.
-- For Claude: provider_session_id == the real Claude session_id (UUID); cwd-bound.
-- ---------------------------------------------------------------------------
create table if not exists workflow_provider_sessions (
  id uuid primary key default gen_random_uuid(),
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  workflow_step_run_id uuid references workflow_run_steps(id) on delete set null,
  provider_key text not null,
  provider_session_id text,
  provider_thread_id text,
  provider_turn_id text,
  transport text,
  working_directory text not null default '',
  model_name text,
  reasoning_effort text,
  capabilities_json jsonb default '{}'::jsonb,
  status text not null default 'starting',
  last_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- One provider session row per (run, provider, cwd) — plain-column unique key so
-- PostgREST on_conflict upsert can target it (working_directory is NOT NULL DEFAULT '').
create unique index if not exists workflow_provider_sessions_run_provider_cwd_uidx
  on workflow_provider_sessions(workflow_run_id, provider_key, working_directory);
create index if not exists workflow_provider_sessions_run_idx
  on workflow_provider_sessions(workflow_run_id);

-- ---------------------------------------------------------------------------
-- Provider events: normalized event telemetry; seq is the monotonic per-run cursor
-- (reconnect/replay via afterSeq, 04-02).
-- ---------------------------------------------------------------------------
create table if not exists workflow_provider_events (
  id uuid primary key default gen_random_uuid(),
  seq bigint not null,
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  workflow_step_run_id uuid references workflow_run_steps(id) on delete set null,
  provider_session_id text,
  provider_key text not null,
  provider_turn_id text,
  event_type text not null,
  payload_json jsonb default '{}'::jsonb,
  occurred_at timestamptz not null default now()
);

-- seq is unique and monotonic within a run (the runner assigns it at a single point).
create unique index if not exists workflow_provider_events_run_seq_uidx
  on workflow_provider_events(workflow_run_id, seq);

-- ---------------------------------------------------------------------------
-- Provider approvals: permission_required records (idempotent, first-write-wins).
-- ---------------------------------------------------------------------------
create table if not exists workflow_provider_approvals (
  id uuid primary key default gen_random_uuid(),
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  workflow_step_run_id uuid references workflow_run_steps(id) on delete set null,
  provider_session_id text,
  provider_key text not null,
  provider_turn_id text,
  request_payload_json jsonb default '{}'::jsonb,
  available_decisions_json jsonb default '[]'::jsonb,
  selected_decision text,
  decided_by text,
  status text not null default 'pending',          -- pending | resolved | expired
  requested_at timestamptz not null default now(),
  decided_at timestamptz,
  expires_at timestamptz
);

create index if not exists workflow_provider_approvals_run_idx
  on workflow_provider_approvals(workflow_run_id);

-- ---------------------------------------------------------------------------
-- Provider questions: the "ask_user" / structured options path.
-- ---------------------------------------------------------------------------
create table if not exists workflow_provider_questions (
  id uuid primary key default gen_random_uuid(),
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  workflow_step_run_id uuid references workflow_run_steps(id) on delete set null,
  provider_session_id text,
  provider_key text not null,
  provider_turn_id text,
  origin text not null default 'ask_user_tool',    -- ask_user_tool | workflow
  prompt text not null,
  options_json jsonb default '[]'::jsonb,
  multi_select boolean not null default false,
  selected_choice_json jsonb,
  status text not null default 'pending',           -- pending | resolved | expired
  requested_at timestamptz not null default now(),
  answered_at timestamptz,
  expires_at timestamptz
);

create index if not exists workflow_provider_questions_run_idx
  on workflow_provider_questions(workflow_run_id);

-- ---------------------------------------------------------------------------
-- RLS: mirror workflow_run_sessions (authenticated read/write). The local runner
-- authenticates with a service-role / RLS-scoped key (see SupabaseWorkflowStore).
-- ---------------------------------------------------------------------------
alter table workflow_provider_sessions  enable row level security;
alter table workflow_provider_events     enable row level security;
alter table workflow_provider_approvals  enable row level security;
alter table workflow_provider_questions  enable row level security;

create policy "authenticated read workflow_provider_sessions"
  on workflow_provider_sessions for select to authenticated using (true);
create policy "authenticated write workflow_provider_sessions"
  on workflow_provider_sessions for all to authenticated using (true) with check (true);

create policy "authenticated read workflow_provider_events"
  on workflow_provider_events for select to authenticated using (true);
create policy "authenticated write workflow_provider_events"
  on workflow_provider_events for all to authenticated using (true) with check (true);

create policy "authenticated read workflow_provider_approvals"
  on workflow_provider_approvals for select to authenticated using (true);
create policy "authenticated write workflow_provider_approvals"
  on workflow_provider_approvals for all to authenticated using (true) with check (true);

create policy "authenticated read workflow_provider_questions"
  on workflow_provider_questions for select to authenticated using (true);
create policy "authenticated write workflow_provider_questions"
  on workflow_provider_questions for all to authenticated using (true) with check (true);
