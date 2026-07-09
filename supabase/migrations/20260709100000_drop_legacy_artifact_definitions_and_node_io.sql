-- Retire the legacy `artifact_definitions` catalog (named-document lineage,
-- e.g. PRD.md path templates) and the raw per-node `inputs_json`/
-- `outputs_json` slot-type declaration. Both predate CP-45's typed artifact
-- framework (`artifact_types`/`artifact_instances`/`step_artifact_bindings`)
-- and, as of this migration, have zero remaining runtime consumer:
--
-- - `artifact_definitions`/`step_input_artifact_definitions`/
--   `step_output_artifact_definitions` were never read by the Go runner
--   (apps/local-runner) and only ever served the desktop app's own
--   WorkflowsSettings.tsx/ArtifactsSettings.tsx "catalog" UI, both removed
--   in this change. The old admin-web app and its Supabase Edge Functions
--   (workflow-engine-start-run / -submit-step-approval / -toggle-yolo-mode)
--   also read these tables, but are confirmed fully retired.
-- - `step_definitions.inputs_json`/`outputs_json` (FlowNode.Inputs/Outputs)
--   had exactly one runtime consumer: resolveEnabledContextSourceIDs'
--   now-retired tier matching a node's declared `outputs:` key against a
--   flow-level `contexts.<name>.sources` binding — superseded by CP-45's
--   artifact-binding tier, which needs neither field.
--
-- `artifact_runs` (the "generated artifacts" feature — still live, desktop
-- ArtifactsSettings.tsx's "Generated" tab) keeps its `artifact_definition_key`
-- column as a plain, no-longer-FK-validated historical string: existing rows
-- keep whatever key they were generated under, but nothing requires that key
-- to still resolve to a catalog row that no longer exists.
begin;

drop table if exists step_input_artifact_definitions;
drop table if exists step_output_artifact_definitions;

alter table artifact_runs
  drop constraint if exists artifact_runs_artifact_definition_key_fkey;

drop table if exists artifact_definitions;

alter table step_definitions
  drop column if exists inputs_json;
alter table step_definitions
  drop column if exists outputs_json;

commit;
