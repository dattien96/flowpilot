-- CP-51 follow-up: enable RLS on durable dispatch tables if the original
-- 20260717120000 migration was already applied without RLS.
-- Idempotent: safe to re-run. Default-deny for anon/authenticated; service_role
-- bypasses RLS. RPCs remain SECURITY DEFINER + execute granted only to service_role.

alter table if exists public.run_protocol_activations enable row level security;
alter table if exists public.dispatch_run_stop_state enable row level security;
alter table if exists public.dispatch_records enable row level security;
alter table if exists public.dispatch_effects enable row level security;
alter table if exists public.repair_records enable row level security;
alter table if exists public.dispatch_intent_clears enable row level security;
alter table if exists public.dispatch_audit enable row level security;

alter table if exists public.run_protocol_activations force row level security;
alter table if exists public.dispatch_run_stop_state force row level security;
alter table if exists public.dispatch_records force row level security;
alter table if exists public.dispatch_effects force row level security;
alter table if exists public.repair_records force row level security;
alter table if exists public.dispatch_intent_clears force row level security;
alter table if exists public.dispatch_audit force row level security;

-- Drop any accidental open policies from manual experiments (none created by us).
do $$
declare
  r record;
begin
  for r in
    select schemaname, tablename, policyname
    from pg_policies
    where schemaname = 'public'
      and tablename in (
        'run_protocol_activations',
        'dispatch_run_stop_state',
        'dispatch_records',
        'dispatch_effects',
        'repair_records',
        'dispatch_intent_clears',
        'dispatch_audit'
      )
  loop
    execute format('drop policy if exists %I on %I.%I', r.policyname, r.schemaname, r.tablename);
  end loop;
end $$;

revoke all on table public.run_protocol_activations from public, anon, authenticated;
revoke all on table public.dispatch_run_stop_state from public, anon, authenticated;
revoke all on table public.dispatch_records from public, anon, authenticated;
revoke all on table public.dispatch_effects from public, anon, authenticated;
revoke all on table public.repair_records from public, anon, authenticated;
revoke all on table public.dispatch_intent_clears from public, anon, authenticated;
revoke all on table public.dispatch_audit from public, anon, authenticated;

grant all on table public.run_protocol_activations to service_role;
grant all on table public.dispatch_run_stop_state to service_role;
grant all on table public.dispatch_records to service_role;
grant all on table public.dispatch_effects to service_role;
grant all on table public.repair_records to service_role;
grant all on table public.dispatch_intent_clears to service_role;
grant all on table public.dispatch_audit to service_role;

-- Ensure RPCs are SECURITY DEFINER and service_role-only (re-apply if 120 was old).
create or replace function public.dispatch_get_run_protocol_version(p_run_id text)
returns int
language sql
stable
security definer
set search_path = public
as $$
  select coalesce(
    (select protocol_version from public.run_protocol_activations where run_id = p_run_id),
    0
  );
$$;

revoke all on function public.dispatch_create_prepared(text, text, jsonb, jsonb) from public, anon, authenticated;
revoke all on function public.dispatch_cas_advance(text, text, bigint, text, text) from public, anon, authenticated;
revoke all on function public.dispatch_request_run_stop(text, bigint, text) from public, anon, authenticated;
revoke all on function public.dispatch_get_run_protocol_version(text) from public, anon, authenticated;

grant execute on function public.dispatch_create_prepared(text, text, jsonb, jsonb) to service_role;
grant execute on function public.dispatch_cas_advance(text, text, bigint, text, text) to service_role;
grant execute on function public.dispatch_request_run_stop(text, bigint, text) to service_role;
grant execute on function public.dispatch_get_run_protocol_version(text) to service_role;
