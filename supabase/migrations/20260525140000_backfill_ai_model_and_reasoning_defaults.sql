begin;

alter table if exists projects
  alter column default_provider set default 'codex',
  alter column default_model set default 'gpt-5.4',
  alter column default_reasoning_effort set default 'medium';

alter table if exists workflows
  alter column provider_override set default 'codex',
  alter column model_override set default 'gpt-5.4',
  alter column reasoning_effort_override set default 'medium';

alter table if exists workflow_steps
  alter column provider_override set default 'codex',
  alter column model_override set default 'gpt-5.4',
  alter column reasoning_effort_override set default 'medium';

update projects
set
  default_model = coalesce(nullif(trim(default_model), ''), 'gpt-5.4'),
  default_reasoning_effort = coalesce(nullif(trim(default_reasoning_effort), ''), 'medium'),
  default_provider = case
    when coalesce(nullif(trim(default_model), ''), 'gpt-5.4') like 'gpt-%' then 'codex'
    when coalesce(nullif(trim(default_model), ''), 'gpt-5.4') like 'gemini-%' then 'gemini'
    when coalesce(nullif(trim(default_model), ''), 'gpt-5.4') like 'claude-%' then 'claude'
    else coalesce(nullif(trim(default_provider), ''), 'codex')
  end;

update workflows
set
  model_override = coalesce(nullif(trim(model_override), ''), 'gpt-5.4'),
  reasoning_effort_override = coalesce(nullif(trim(reasoning_effort_override), ''), 'medium'),
  provider_override = case
    when coalesce(nullif(trim(model_override), ''), 'gpt-5.4') like 'gpt-%' then 'codex'
    when coalesce(nullif(trim(model_override), ''), 'gpt-5.4') like 'gemini-%' then 'gemini'
    when coalesce(nullif(trim(model_override), ''), 'gpt-5.4') like 'claude-%' then 'claude'
    else coalesce(nullif(trim(provider_override), ''), 'codex')
  end;

update workflow_steps
set
  model_override = coalesce(nullif(trim(model_override), ''), 'gpt-5.4'),
  reasoning_effort_override = coalesce(nullif(trim(reasoning_effort_override), ''), 'medium'),
  provider_override = case
    when coalesce(nullif(trim(model_override), ''), 'gpt-5.4') like 'gpt-%' then 'codex'
    when coalesce(nullif(trim(model_override), ''), 'gpt-5.4') like 'gemini-%' then 'gemini'
    when coalesce(nullif(trim(model_override), ''), 'gpt-5.4') like 'claude-%' then 'claude'
    else coalesce(nullif(trim(provider_override), ''), 'codex')
  end;

commit;
