-- Add persisted subprocess PID for workflow run sessions.

alter table workflow_run_sessions
  add column if not exists process_pid integer;
