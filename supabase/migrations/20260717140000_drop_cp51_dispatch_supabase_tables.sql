-- Task-258 / CP-51: drop Supabase dispatch objects from the early CP-51 turn.
-- Product path is local per-project NDJSON + Google Drive sync, not these tables.
-- Safe to run if 20260717120000 / 20260717130000 were applied; no-op if objects missing.

-- RPCs first (may be SECURITY DEFINER).
drop function if exists public.dispatch_create_prepared(text, text, jsonb, jsonb);
drop function if exists public.dispatch_cas_advance(text, text, bigint, text, text);
drop function if exists public.dispatch_request_run_stop(text, bigint, text);
drop function if exists public.dispatch_get_run_protocol_version(text);

-- Dependent / leaf tables first, then parents.
drop table if exists public.dispatch_audit cascade;
drop table if exists public.dispatch_intent_clears cascade;
drop table if exists public.repair_records cascade;
drop table if exists public.dispatch_effects cascade;
drop table if exists public.dispatch_records cascade;
drop table if exists public.dispatch_run_stop_state cascade;
drop table if exists public.run_protocol_activations cascade;
