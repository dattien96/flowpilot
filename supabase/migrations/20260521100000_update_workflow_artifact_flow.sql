insert into artifact_definitions (
  key,
  name,
  description,
  local_path_template,
  remote_path_template,
  default_file_name,
  created_at,
  updated_at
) values (
  'coding_implementation_checklist_artifact',
  'Coding Implementation Checklist',
  'Coding implementation checklist artifact',
  '.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/CodingImplementationChecklist.md',
  'artifacts/{projectId}/{workflowRunId}/{stepType}/CodingImplementationChecklist.md',
  'CodingImplementationChecklist.md',
  now(),
  now()
)
on conflict (key) do update
  set name = excluded.name,
      description = excluded.description,
      local_path_template = excluded.local_path_template,
      remote_path_template = excluded.remote_path_template,
      default_file_name = excluded.default_file_name,
      updated_at = now();

delete from step_input_artifact_definitions
where (step_type, artifact_definition_key) in (
  ('create_architecture', 'coding_plan_artifact')
);

insert into step_input_artifact_definitions (step_type, artifact_definition_key, order_index)
values
  ('create_architecture', 'business_summary_artifact', 0),
  ('code_review_loop', 'coding_plan_artifact', 0),
  ('code_review_loop', 'tdd_plan_artifact', 1)
on conflict (step_type, artifact_definition_key) do update
  set order_index = excluded.order_index;

insert into step_output_artifact_definitions (step_type, artifact_definition_key, order_index)
values
  ('code_review_loop', 'coding_implementation_checklist_artifact', 0),
  ('code_review_loop', 'code_review_summary_artifact', 1)
on conflict (step_type, artifact_definition_key) do update
  set order_index = excluded.order_index;
