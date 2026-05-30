-- Migration: Add ai_supported_models table and seed initial models

create table if not exists public.ai_supported_models (
  id uuid primary key default gen_random_uuid(),
  provider_key text not null check (provider_key in ('codex', 'claude', 'gemini')),
  model_id text not null unique,
  display_name text not null,
  is_enabled boolean not null default true,
  sort_order integer not null default 0,
  source text not null default 'manual',
  detection_method text null,
  detected_cli_version text null,
  last_detected_at timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- Enable RLS
alter table public.ai_supported_models enable row level security;

-- Drop existing policies if any to prevent duplicate creation failures
drop policy if exists "ai_supported_models_select_authenticated" on public.ai_supported_models;
drop policy if exists "ai_supported_models_write_authenticated" on public.ai_supported_models;

-- RLS Policies
create policy "ai_supported_models_select_authenticated"
  on public.ai_supported_models for select to authenticated using (true);

create policy "ai_supported_models_write_authenticated"
  on public.ai_supported_models for all to authenticated using (true) with check (true);

-- Seed current models
insert into public.ai_supported_models (provider_key, model_id, display_name, source, sort_order) values
  ('gemini', 'auto-gemini-3', 'Auto (Gemini 3)', 'seed', 1),
  ('gemini', 'auto-gemini-2.5', 'Auto (Gemini 2.5)', 'seed', 2),
  ('gemini', 'gemini-3.1-pro-preview', 'Gemini 3.1 Pro Preview', 'seed', 3),
  ('gemini', 'gemini-3-flash-preview', 'Gemini 3 Flash Preview', 'seed', 4),
  ('gemini', 'gemini-3.1-flash-lite-preview', 'Gemini 3.1 Flash Lite Preview', 'seed', 5),
  ('gemini', 'gemini-2.5-pro', 'Gemini 2.5 Pro', 'seed', 6),
  ('gemini', 'gemini-2.5-flash', 'Gemini 2.5 Flash', 'seed', 7),
  ('gemini', 'gemini-2.5-flash-lite', 'Gemini 2.5 Flash Lite', 'seed', 8),
  ('claude', 'claude-haiku', 'Claude Haiku', 'seed', 9),
  ('claude', 'claude-sonnet', 'Claude Sonnet', 'seed', 10),
  ('claude', 'claude-opus', 'Claude Opus', 'seed', 11),
  ('codex', 'gpt-5.4-mini', 'GPT 5.4 Mini', 'seed', 12),
  ('codex', 'gpt-5.4', 'GPT 5.4', 'seed', 13),
  ('codex', 'gpt-5.5', 'GPT 5.5', 'seed', 14)
on conflict (model_id) do update set
  display_name = excluded.display_name,
  provider_key = excluded.provider_key,
  sort_order = excluded.sort_order,
  source = excluded.source;

-- Drop constraint if exists on step_definitions
alter table public.step_definitions
  drop constraint if exists step_definitions_supported_model_check;
