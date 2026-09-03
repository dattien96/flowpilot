-- CP-59 / SD-26 Chat SSOT (Task-313 DOD-10): additive migration.
-- 1) Chat leg columns on workflow_provider_sessions (append-only; existing
--    rows stay NULL — legacy chats self-tag lazily in the runner).
-- 2) workflow_chat_events: the durable per-chat timeline (SD26-D-1). Append is
--    idempotent on unique(chat_id, chat_seq) so restore replays act as upserts.

ALTER TABLE workflow_provider_sessions
  ADD COLUMN IF NOT EXISTS chat_id text;
ALTER TABLE workflow_provider_sessions
  ADD COLUMN IF NOT EXISTS leg_seq bigint;
ALTER TABLE workflow_provider_sessions
  ADD COLUMN IF NOT EXISTS leg_state text;
ALTER TABLE workflow_provider_sessions
  ADD COLUMN IF NOT EXISTS leg_closed_reason text;
ALTER TABLE workflow_provider_sessions
  ADD COLUMN IF NOT EXISTS switch_from_run_id text;

CREATE TABLE IF NOT EXISTS workflow_chat_events (
  chat_id text NOT NULL,
  chat_seq bigint NOT NULL,
  leg_run_id text NOT NULL,
  type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (chat_id, chat_seq)
);

CREATE INDEX IF NOT EXISTS workflow_chat_events_chat_leg_idx
  ON workflow_chat_events (chat_id, leg_run_id);
