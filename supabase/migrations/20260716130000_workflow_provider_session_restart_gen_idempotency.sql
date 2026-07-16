-- BUG-288 R16-P0: durable stall-retry generation + startTurn idempotency keys
-- on workflow_provider_sessions (mirrors LocalFileSessionStore).

alter table if exists workflow_provider_sessions
  add column if not exists pending_restart_gen bigint not null default 0;
alter table if exists workflow_provider_sessions
  add column if not exists idempotency_keys jsonb not null default '{}'::jsonb;
