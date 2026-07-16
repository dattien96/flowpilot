-- BUG-288 R18-3: session_runtime jsonb holds full flow-recovery fields
-- (PendingFlowGate*, FlowCohortID, topology, resume/reprompt intents, etc.)
-- so Supabase ListAllProviderSessions can reconstruct parent+child flows.

alter table if exists workflow_provider_sessions
  add column if not exists session_runtime jsonb not null default '{}'::jsonb;
