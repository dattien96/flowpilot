alter table step_definitions
  add column if not exists prompt_base text;

update step_definitions
set prompt_base = concat('You are executing the "', name, '" workflow step.', E'\n\n', description)
where prompt_base is null or btrim(prompt_base) = '';

update step_definitions
set model = 'gpt-5.5'
where model is null or btrim(model) = '';

alter table step_definitions
  alter column model set default 'gpt-5.5';

alter table step_definitions
  drop constraint if exists step_definitions_supported_model_check;

alter table step_definitions
  add constraint step_definitions_supported_model_check
  check (
    model in (
      'gemini-flash',
      'gemini-pro',
      'claude-haiku',
      'claude-sonnet',
      'claude-opus',
      'gpt-5.4-mini',
      'gpt-5.4',
      'gpt-5.5'
    )
  );
