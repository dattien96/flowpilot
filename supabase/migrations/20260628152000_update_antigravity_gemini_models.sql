-- Update Gemini model inventory to the Antigravity CLI lineup.

create or replace function public.normalize_gemini_model_20260628(model_value text)
returns text
language sql
immutable
as $$
  select case lower(trim(coalesce(model_value, '')))
    when 'flash' then 'gemini-3.5-flash-medium'
    when 'gemini-flash' then 'gemini-3.5-flash-medium'
    when 'pro' then 'gemini-3.1-pro-high'
    when 'gemini-pro' then 'gemini-3.1-pro-high'
    when 'auto-gemini-3' then 'gemini-3.5-flash-medium'
    when 'auto-gemini-2.5' then 'gemini-3.5-flash-medium'
    when 'gemini-3-pro-preview' then 'gemini-3.1-pro-high'
    when 'gemini-3.1-pro-preview' then 'gemini-3.1-pro-high'
    when 'gemini-3.1-pro-preview-customtools' then 'gemini-3.1-pro-high'
    when 'gemini-3-flash-preview' then 'gemini-3.5-flash-medium'
    when 'gemini-3.1-flash-lite-preview' then 'gemini-3.5-flash-low'
    when 'gemini-2.5-pro' then 'gemini-3.1-pro-high'
    when 'gemini-2.5-flash' then 'gemini-3.5-flash-medium'
    when 'gemini-2.5-flash-lite' then 'gemini-3.5-flash-low'
    else model_value
  end
$$;

update public.projects
set default_model = public.normalize_gemini_model_20260628(default_model)
where default_model is not null;

update public.workflows
set model_override = public.normalize_gemini_model_20260628(model_override)
where model_override is not null;

update public.workflow_steps
set model_override = public.normalize_gemini_model_20260628(model_override)
where model_override is not null;

update public.step_definitions
set model = public.normalize_gemini_model_20260628(model)
where model is not null;

delete from public.ai_supported_models
where provider_key = 'gemini'
  and source = 'seed';

insert into public.ai_supported_models (provider_key, model_id, display_name, source, sort_order) values
  ('gemini', 'gemini-3.5-flash-medium', 'Gemini 3.5 Flash (Medium)', 'seed', 1),
  ('gemini', 'gemini-3.5-flash-high', 'Gemini 3.5 Flash (High)', 'seed', 2),
  ('gemini', 'gemini-3.5-flash-low', 'Gemini 3.5 Flash (Low)', 'seed', 3),
  ('gemini', 'gemini-3.1-pro-low', 'Gemini 3.1 Pro (Low)', 'seed', 4),
  ('gemini', 'gemini-3.1-pro-high', 'Gemini 3.1 Pro (High)', 'seed', 5)
on conflict (model_id) do update set
  display_name = excluded.display_name,
  provider_key = excluded.provider_key,
  sort_order = excluded.sort_order,
  source = excluded.source;

alter table public.step_definitions
  drop constraint if exists step_definitions_supported_model_check;

alter table public.step_definitions
  add constraint step_definitions_supported_model_check
  check (
    model in (
      'gemini-3.5-flash-medium',
      'gemini-3.5-flash-high',
      'gemini-3.5-flash-low',
      'gemini-3.1-pro-low',
      'gemini-3.1-pro-high',
      'gemini-flash',
      'gemini-pro',
      'auto-gemini-3',
      'auto-gemini-2.5',
      'gemini-3-pro-preview',
      'gemini-3.1-pro-preview',
      'gemini-3.1-pro-preview-customtools',
      'gemini-3-flash-preview',
      'gemini-3.1-flash-lite-preview',
      'gemini-2.5-pro',
      'gemini-2.5-flash',
      'gemini-2.5-flash-lite',
      'claude-haiku',
      'claude-sonnet',
      'claude-opus',
      'gpt-5.4-mini',
      'gpt-5.4',
      'gpt-5.5'
    )
  );

drop function public.normalize_gemini_model_20260628(text);
