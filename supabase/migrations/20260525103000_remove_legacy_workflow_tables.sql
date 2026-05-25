-- Remove legacy workflow-definition/output/approval tables now that the app
-- reads workflow definitions from `workflows`/`workflow_steps` and approval
-- state from canonical `workflow_run_steps` plus `workflow_run_logs`.

begin;

alter table if exists public.ai_call_logs
  drop constraint if exists ai_call_logs_workflow_run_id_fkey;

alter table if exists public.ai_call_logs
  drop constraint if exists ai_call_logs_workflow_step_id_fkey;

drop table if exists public.approval_decisions;
drop table if exists public.approvals;
drop table if exists public.ai_outputs;
drop table if exists public.legacy_workflow_steps;
drop table if exists public.legacy_workflow_runs;
drop table if exists public.workflow_definitions;

commit;
