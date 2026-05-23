-- Admin app access is based on authenticated Supabase users, not project-team membership.

drop policy if exists "authenticated read artifact_runs" on artifact_runs;
create policy "authenticated read artifact_runs"
  on artifact_runs for select to authenticated using (true);
create policy "authenticated write artifact_runs"
  on artifact_runs for all to authenticated using (true) with check (true);

drop policy if exists "ai_prompt_templates_select_authenticated" on ai_prompt_templates;
drop policy if exists "ai_prompt_templates_insert_authenticated" on ai_prompt_templates;
drop policy if exists "ai_prompt_templates_update_project_members" on ai_prompt_templates;
drop policy if exists "ai_prompt_templates_delete_project_members" on ai_prompt_templates;
create policy "ai_prompt_templates_read_authenticated"
  on ai_prompt_templates for select to authenticated using (true);
create policy "ai_prompt_templates_write_authenticated"
  on ai_prompt_templates for all to authenticated using (true) with check (true);

drop policy if exists "ai_runs_select_project_members" on ai_runs;
drop policy if exists "ai_runs_insert_authenticated" on ai_runs;
drop policy if exists "ai_runs_update_project_members" on ai_runs;
drop policy if exists "ai_runs_delete_project_members" on ai_runs;
create policy "ai_runs_read_authenticated"
  on ai_runs for select to authenticated using (true);
create policy "ai_runs_write_authenticated"
  on ai_runs for all to authenticated using (true) with check (true);

drop policy if exists "artifact_annotations_select" on artifact_annotations;
drop policy if exists "artifact_annotations_insert" on artifact_annotations;
drop policy if exists "artifact_annotations_update" on artifact_annotations;
drop policy if exists "artifact_annotations_delete" on artifact_annotations;
create policy "artifact_annotations_read_authenticated"
  on artifact_annotations for select to authenticated using (true);
create policy "artifact_annotations_write_authenticated"
  on artifact_annotations for all to authenticated using (true) with check (true);
