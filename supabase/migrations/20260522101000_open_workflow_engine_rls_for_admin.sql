-- Fix: Replace team_members-based RLS with open authenticated access on all
-- workflow engine tables. This is the admin app; all authenticated users are admins.

-- workflows
drop policy if exists "authenticated read workflows" on workflows;
drop policy if exists "authenticated write private workflows" on workflows;
drop policy if exists "authenticated write global workflows" on workflows;

create policy "authenticated read workflows"
  on workflows for select to authenticated using (true);
create policy "authenticated write workflows"
  on workflows for all to authenticated using (true) with check (true);

-- workflow_steps
drop policy if exists "authenticated read workflow_steps" on workflow_steps;
drop policy if exists "authenticated write private workflow_steps" on workflow_steps;
drop policy if exists "authenticated write global workflow_steps" on workflow_steps;

create policy "authenticated read workflow_steps"
  on workflow_steps for select to authenticated using (true);
create policy "authenticated write workflow_steps"
  on workflow_steps for all to authenticated using (true) with check (true);

-- workflow_prompt_cache
drop policy if exists "authenticated read workflow_prompt_cache" on workflow_prompt_cache;
drop policy if exists "authenticated write workflow_prompt_cache" on workflow_prompt_cache;

create policy "authenticated read workflow_prompt_cache"
  on workflow_prompt_cache for select to authenticated using (true);
create policy "authenticated write workflow_prompt_cache"
  on workflow_prompt_cache for all to authenticated using (true) with check (true);

-- workflow_runs
drop policy if exists "authenticated read workflow_runs" on workflow_runs;
drop policy if exists "authenticated write workflow_runs" on workflow_runs;

create policy "authenticated read workflow_runs"
  on workflow_runs for select to authenticated using (true);
create policy "authenticated write workflow_runs"
  on workflow_runs for all to authenticated using (true) with check (true);

-- workflow_run_steps
drop policy if exists "authenticated read workflow_run_steps" on workflow_run_steps;
drop policy if exists "authenticated write workflow_run_steps" on workflow_run_steps;

create policy "authenticated read workflow_run_steps"
  on workflow_run_steps for select to authenticated using (true);
create policy "authenticated write workflow_run_steps"
  on workflow_run_steps for all to authenticated using (true) with check (true);

-- workflow_run_logs
drop policy if exists "authenticated read workflow_run_logs" on workflow_run_logs;
drop policy if exists "authenticated write workflow_run_logs" on workflow_run_logs;

create policy "authenticated read workflow_run_logs"
  on workflow_run_logs for select to authenticated using (true);
create policy "authenticated write workflow_run_logs"
  on workflow_run_logs for all to authenticated using (true) with check (true);
