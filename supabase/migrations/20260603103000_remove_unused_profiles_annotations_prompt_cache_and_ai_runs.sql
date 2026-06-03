begin;

-- ============================================================================
-- Remove artifact_annotations
-- ============================================================================

drop policy if exists "artifact_annotations_read_authenticated" on public.artifact_annotations;
drop policy if exists "artifact_annotations_write_authenticated" on public.artifact_annotations;
drop policy if exists "artifact_annotations_select" on public.artifact_annotations;
drop policy if exists "artifact_annotations_insert" on public.artifact_annotations;
drop policy if exists "artifact_annotations_update" on public.artifact_annotations;
drop policy if exists "artifact_annotations_delete" on public.artifact_annotations;

drop table if exists public.artifact_annotations;

-- ============================================================================
-- Remove profiles
-- ============================================================================

drop policy if exists "authenticated read profiles" on public.profiles;

drop table if exists public.profiles;

-- ============================================================================
-- Remove workflow prompt cache plumbing
-- ============================================================================

alter table if exists public.workflow_run_steps
  drop column if exists prompt_cache_id;

drop policy if exists "authenticated read workflow_prompt_cache" on public.workflow_prompt_cache;
drop policy if exists "authenticated write workflow_prompt_cache" on public.workflow_prompt_cache;

drop table if exists public.workflow_prompt_cache;

-- ============================================================================
-- Remove ai_runs
-- Note: this also removes the current AI Runs admin surface until app code is
-- cleaned up to stop querying public.ai_runs.
-- ============================================================================

drop policy if exists "ai_runs_read_authenticated" on public.ai_runs;
drop policy if exists "ai_runs_write_authenticated" on public.ai_runs;
drop policy if exists "ai_runs_select_project_members" on public.ai_runs;
drop policy if exists "ai_runs_insert_authenticated" on public.ai_runs;
drop policy if exists "ai_runs_update_project_members" on public.ai_runs;
drop policy if exists "ai_runs_delete_project_members" on public.ai_runs;

drop table if exists public.ai_runs;

commit;
