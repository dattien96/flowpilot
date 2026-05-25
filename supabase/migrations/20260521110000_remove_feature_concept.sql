alter table if exists context_sources
  drop constraint if exists context_sources_feature_id_fkey;

alter table if exists workflow_runs
  drop constraint if exists workflow_runs_feature_id_fkey;

alter table if exists legacy_workflow_runs
  drop constraint if exists workflow_runs_feature_id_fkey;

alter table if exists ai_outputs
  drop constraint if exists ai_outputs_feature_id_fkey;

drop index if exists context_sources_feature_id_idx;
drop index if exists workflow_runs_feature_id_idx;
drop index if exists features_project_id_idx;

alter table if exists context_sources
  drop column if exists feature_id;

alter table if exists workflow_runs
  drop column if exists feature_id;

alter table if exists legacy_workflow_runs
  drop column if exists feature_id;

alter table if exists ai_outputs
  drop column if exists feature_id;

drop table if exists features;
