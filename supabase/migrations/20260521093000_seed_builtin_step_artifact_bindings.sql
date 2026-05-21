insert into step_output_artifact_definitions (step_type, artifact_definition_key, order_index)
values
  ('business_idea', 'business_idea_artifact', 0),
  ('feature_intake', 'feature_intake_artifact', 0),
  ('business_summary', 'business_summary_artifact', 0),
  ('product_spec', 'product_spec_artifact', 0),
  ('tech_spec', 'tech_spec_artifact', 0),
  ('make_plan_coding', 'coding_plan_artifact', 0),
  ('create_architecture', 'architecture_artifact', 0),
  ('tdd', 'tdd_plan_artifact', 0),
  ('task_breakdown', 'task_breakdown_artifact', 0),
  ('code_review_loop', 'code_review_summary_artifact', 0),
  ('release_readiness', 'release_readiness_artifact', 0),
  ('issue_analysis', 'root_cause_analysis_artifact', 0),
  ('analytics_review', 'usage_analytics_artifact', 0),
  ('project_analysis', 'project_analysis_artifact', 0),
  ('code_traceability', 'code_traceability_artifact', 0)
on conflict (step_type, artifact_definition_key) do update
  set order_index = excluded.order_index;

insert into step_input_artifact_definitions (step_type, artifact_definition_key, order_index)
values
  ('business_summary', 'business_idea_artifact', 0),
  ('business_summary', 'feature_intake_artifact', 1),
  ('product_spec', 'business_summary_artifact', 0),
  ('make_plan_coding', 'tech_spec_artifact', 0),
  ('create_architecture', 'coding_plan_artifact', 0),
  ('tdd', 'tech_spec_artifact', 0),
  ('tdd', 'coding_plan_artifact', 1),
  ('tdd', 'architecture_artifact', 2),
  ('code_review_loop', 'coding_plan_artifact', 0),
  ('release_readiness', 'code_review_summary_artifact', 0)
on conflict (step_type, artifact_definition_key) do update
  set order_index = excluded.order_index;
