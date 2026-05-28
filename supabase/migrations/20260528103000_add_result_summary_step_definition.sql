insert into step_definitions (
  step_type,
  name,
  description,
  required_mcps,
  required_skills,
  agent_type,
  prompt_base,
  model,
  reasoning_effort
)
values (
  'result_summary',
  'Summary',
  'Runtime-generated final workflow summary step.',
  '[]'::jsonb,
  '[]'::jsonb,
  'standard',
  'You are a workflow summarizer. Read the earlier completed workflow step outputs and produce one concise final summary covering the overall objective, completed work, key decisions, deliverables, and remaining risks or follow-up items.',
  'gpt-5.4-mini',
  'low'
)
on conflict (step_type) do update set
  name = excluded.name,
  description = excluded.description,
  required_mcps = excluded.required_mcps,
  required_skills = excluded.required_skills,
  agent_type = excluded.agent_type,
  prompt_base = excluded.prompt_base,
  model = excluded.model,
  reasoning_effort = excluded.reasoning_effort;
