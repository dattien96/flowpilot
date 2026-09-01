-- CP-58 Task-307: built-in file_artifact.v1 instances for the harness plan
-- outputs (plan_md / cp_md / task_md).
--
-- `is_builtin=true`, `project_id=null` (global, visible from every project —
-- mirrors built-in workflows; RLS keeps `authenticated` read-only on these
-- rows, same pattern as the Task-201 default context instance).
--
-- `config_json.pathTemplate` is the Task-307 naming contract: the writer
-- resolves `{{idx}}` (next free number in the target folder) and `{{slug}}`
-- (short kebab-case slug) at authoring time. Templates are prompt-contract
-- only in the runner (appendTemplatedFileArtifactOutputsPrompt,
-- artifact_type_registry.go) and deliberately NOT part of the flowgate
-- exact-path check; the agentpack loader validates at flow-load time that
-- every file_artifact OUTPUT path stays under `requirements/`
-- (validateArtifactOutputPath, pack.go).
--
-- Fixed, well-known ids (continuing the Task-201 sequence) so mirror-sync and
-- step_artifact_bindings rows can reference these instances deterministically
-- without a lookup round-trip. The embedded FS pack declares its bindings by
-- the stable slot names (plan_md/cp_md/task_md) while these rows are the
-- Supabase-side catalog for the artifact panel.
insert into artifact_instances (
  id,
  project_id,
  artifact_type_id,
  name,
  description,
  config_json,
  is_builtin,
  status
) values
(
  '00000000-0000-0000-0000-000000000002',
  null,
  'file_artifact.v1',
  'plan_md',
  'Task plan md written by task-harness plan_writer (CP-58).',
  '{"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md", "required": true, "description": "Task plan md from task-harness plan_writer"}'::jsonb,
  true,
  'active'
),
(
  '00000000-0000-0000-0000-000000000003',
  null,
  'file_artifact.v1',
  'cp_md',
  'Coding plan md written by cp-harness cp_plan_writer (CP-58).',
  '{"pathTemplate": "requirements/07-Coding-Plan/todo/CP-{{cpID}}-{{slug}}.md", "required": true, "description": "CP architecture md from cp-harness cp_plan_writer"}'::jsonb,
  true,
  'active'
),
(
  '00000000-0000-0000-0000-000000000004',
  null,
  'file_artifact.v1',
  'task_md',
  'Individual Task md files written by cp-harness task_splitter, one per P-* (CP-58).',
  '{"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md", "required": true, "count": "N per P-*", "description": "Individual Task md files from cp-harness task_splitter"}'::jsonb,
  true,
  'active'
)
on conflict (id) do update
  set name = excluded.name,
      description = excluded.description,
      config_json = excluded.config_json,
      is_builtin = true,
      status = excluded.status,
      updated_at = now();
