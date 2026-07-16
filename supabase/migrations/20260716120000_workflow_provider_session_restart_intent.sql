-- BUG-288 R13-01 / R13-16: durable stall-Retry restart intent + flow-context inject flag
-- on workflow_provider_sessions (mirrors LocalFileSessionStore ndjson fields).

alter table if exists workflow_provider_sessions
  add column if not exists pending_restart_run_id text;
alter table if exists workflow_provider_sessions
  add column if not exists pending_restart_prompt text;
alter table if exists workflow_provider_sessions
  add column if not exists flow_context_injected boolean not null default false;
