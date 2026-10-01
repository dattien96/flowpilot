-- CA-1079: built-in file_artifact.v1 instance for vibe-sprint's `tdd` node
-- output (slot tdd_signatures) — the Go-side binding seed
-- (builtinHarnessTddSignaturesInstanceID, builtin_artifact_bindings.go)
-- references 00000000-0000-0000-0000-000000000005 by literal but no
-- migration ever shipped the row, so the step_artifact_bindings upsert
-- FK-violated (23503) and dropped the whole batch on remotes.
--
-- Same shape as 20260831090000_add_harness_plan_artifact_instances.sql:
-- is_builtin=true, project_id=null (global), pathTemplate continues the
-- Task-307 naming contract. The on-conflict update keeps the row convergent
-- on re-run.

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
  '00000000-0000-0000-0000-000000000005',
  null,
  'file_artifact.v1',
  'tdd_signatures',
  'TDD signature list written by vibe-sprint tdd (scaffold-architect) — the RED-test contract coder must satisfy (CP-67).',
  '{"pathTemplate": "requirements/.flowpilot/vibe/tdd-signatures.md", "required": true, "description": "TDD signature artifact from vibe-sprint tdd"}'::jsonb,
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
