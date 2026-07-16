-- CP-51 / SD-24: durable turn dispatch tables + transactional RPCs (core subset).
-- Full parity with local DispatchStore; runner client calls these via PostgREST rpc/.

-- Non-prunable run protocol activation (V2 authority).
create table if not exists public.run_protocol_activations (
  run_id text primary key,
  protocol_version int not null default 2,
  created_at timestamptz not null default now()
);

-- Sole run-Stop authority for send fences.
create table if not exists public.dispatch_run_stop_state (
  run_id text primary key,
  generation bigint not null default 0,
  revision bigint not null default 1,
  stopped boolean not null default false,
  updated_at timestamptz not null default now()
);

create table if not exists public.dispatch_records (
  run_id text not null,
  turn_id text not null,
  protocol_version int not null default 2,
  intent_owner_run_id text,
  state text not null,
  stop_outcome text,
  revision bigint not null default 1,
  claim_owner text,
  claim_expires_at timestamptz,
  recovery_attach_epoch bigint not null default 0,
  recovery_attach_owner text,
  recovery_attach_expires_at timestamptz,
  cancel_requested boolean not null default false,
  stop_generation bigint not null default 0,
  outer_intent_key text,
  outer_intent_gen bigint not null default 0,
  envelope_hash text not null,
  envelope jsonb,
  receipt_evidence jsonb,
  terminal_evidence jsonb,
  settle_owed boolean not null default false,
  settle_phase text,
  predecessor_turn_id text,
  outcome text,
  parent_stop_fence jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (run_id, turn_id)
);

create index if not exists dispatch_records_state_idx on public.dispatch_records (state);
create index if not exists dispatch_records_outer_intent_idx
  on public.dispatch_records (run_id, outer_intent_key, outer_intent_gen);

create table if not exists public.dispatch_effects (
  run_id text not null,
  turn_id text not null,
  effect_kind text not null,
  payload jsonb,
  payload_hash text not null,
  revision bigint not null default 1,
  state text,
  stop_generation bigint,
  created_at timestamptz not null default now(),
  primary key (run_id, turn_id, effect_kind)
);

create table if not exists public.repair_records (
  run_id text not null,
  repair_revision bigint not null default 1,
  reason text not null,
  quarantine_blob bytea,
  quarantine_hash text not null,
  state text not null default 'open',
  resolved_action text,
  resolution_id text,
  attempt_claim text,
  attempt_expires_at timestamptz,
  created_at timestamptz not null default now(),
  resolved_at timestamptz,
  primary key (run_id, repair_revision)
);

create unique index if not exists repair_records_one_open_uidx
  on public.repair_records (run_id) where state = 'open';

create table if not exists public.dispatch_intent_clears (
  owner_run_id text not null,
  intent_key text not null,
  intent_gen bigint not null,
  cleared_at timestamptz not null default now(),
  primary key (owner_run_id, intent_key, intent_gen)
);

create table if not exists public.dispatch_audit (
  seq bigserial primary key,
  run_id text not null,
  turn_id text,
  kind text not null,
  detail text,
  actor text,
  at timestamptz not null default now()
);

-- Create prepared + atomic V2 activation + run stop state.
create or replace function public.dispatch_create_prepared(
  p_run_id text,
  p_turn_id text,
  p_record jsonb,
  p_envelope jsonb
) returns jsonb
language plpgsql
security definer
set search_path = public
as $$
declare
  existing public.dispatch_records%rowtype;
begin
  insert into public.run_protocol_activations (run_id, protocol_version)
  values (p_run_id, 2)
  on conflict (run_id) do nothing;

  insert into public.dispatch_run_stop_state (run_id)
  values (p_run_id)
  on conflict (run_id) do nothing;

  select * into existing from public.dispatch_records
  where run_id = p_run_id and turn_id = p_turn_id;
  if found then
    return to_jsonb(existing);
  end if;

  insert into public.dispatch_records (
    run_id, turn_id, protocol_version, intent_owner_run_id, state, revision,
    outer_intent_key, outer_intent_gen, envelope_hash, envelope, settle_owed, settle_phase
  ) values (
    p_run_id,
    p_turn_id,
    coalesce((p_record->>'protocol_version')::int, 2),
    coalesce(p_record->>'intent_owner_run_id', p_run_id),
    coalesce(p_record->>'state', 'prepared'),
    1,
    p_record->>'outer_intent_key',
    coalesce((p_record->>'outer_intent_gen')::bigint, 0),
    coalesce(p_record->>'envelope_hash', p_envelope->>'envelope_hash', ''),
    p_envelope,
    coalesce((p_record->>'settle_owed')::boolean, false),
    nullif(p_record->>'settle_phase', '')
  )
  returning * into existing;

  insert into public.dispatch_audit (run_id, turn_id, kind, detail, actor)
  values (p_run_id, p_turn_id, 'create_prepared', 'state=prepared', 'system');

  return to_jsonb(existing);
end;
$$;

-- CAS advance with optional send_started fence against run stop.
create or replace function public.dispatch_cas_advance(
  p_run_id text,
  p_turn_id text,
  p_expected_rev bigint,
  p_expected_state text,
  p_next_state text
) returns jsonb
language plpgsql
security definer
set search_path = public
as $$
declare
  rec public.dispatch_records%rowtype;
  st public.dispatch_run_stop_state%rowtype;
begin
  select * into rec from public.dispatch_records
  where run_id = p_run_id and turn_id = p_turn_id for update;
  if not found then
    raise exception 'not_found' using errcode = 'P0002';
  end if;
  if rec.revision <> p_expected_rev or rec.state <> p_expected_state then
    raise exception 'stale' using errcode = 'P0001';
  end if;

  if p_next_state = 'send_started' then
    select * into st from public.dispatch_run_stop_state where run_id = p_run_id for update;
    if found and st.stopped then
      raise exception 'run_stop_fence:%', st.generation using errcode = 'P0003';
    end if;
    if rec.cancel_requested then
      raise exception 'cancel_blocks_send' using errcode = 'P0004';
    end if;
  end if;

  update public.dispatch_records set
    state = p_next_state,
    revision = revision + 1,
    updated_at = now()
  where run_id = p_run_id and turn_id = p_turn_id
  returning * into rec;

  insert into public.dispatch_audit (run_id, turn_id, kind, detail, actor)
  values (p_run_id, p_turn_id, 'cas_advance', p_expected_state || '->' || p_next_state, 'system');

  return to_jsonb(rec);
end;
$$;

create or replace function public.dispatch_request_run_stop(
  p_run_id text,
  p_expected_rev bigint,
  p_reason text
) returns jsonb
language plpgsql
security definer
set search_path = public
as $$
declare
  st public.dispatch_run_stop_state%rowtype;
begin
  select * into st from public.dispatch_run_stop_state where run_id = p_run_id for update;
  if not found then
    insert into public.dispatch_run_stop_state (run_id) values (p_run_id)
    returning * into st;
  end if;
  if st.revision <> p_expected_rev then
    raise exception 'stale' using errcode = 'P0001';
  end if;
  update public.dispatch_run_stop_state set
    stopped = true,
    generation = generation + 1,
    revision = revision + 1,
    updated_at = now()
  where run_id = p_run_id
  returning * into st;
  insert into public.dispatch_audit (run_id, kind, detail, actor)
  values (p_run_id, 'request_run_stop', coalesce(p_reason, 'user'), 'system');
  return to_jsonb(st);
end;
$$;

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

-- ---------------------------------------------------------------------------
-- RLS: runner-owned durable state. Clients using anon/authenticated must NOT
-- read or write these tables. The local-runner uses the service_role key, which
-- bypasses RLS. SECURITY DEFINER RPCs above are executable only by service_role.
-- No policies for anon/authenticated = default-deny under RLS.
-- ---------------------------------------------------------------------------

alter table public.run_protocol_activations enable row level security;
alter table public.dispatch_run_stop_state enable row level security;
alter table public.dispatch_records enable row level security;
alter table public.dispatch_effects enable row level security;
alter table public.repair_records enable row level security;
alter table public.dispatch_intent_clears enable row level security;
alter table public.dispatch_audit enable row level security;

-- Force RLS for table owner as well (defense-in-depth for non-superuser owners).
alter table public.run_protocol_activations force row level security;
alter table public.dispatch_run_stop_state force row level security;
alter table public.dispatch_records force row level security;
alter table public.dispatch_effects force row level security;
alter table public.repair_records force row level security;
alter table public.dispatch_intent_clears force row level security;
alter table public.dispatch_audit force row level security;

-- Explicit grants: strip public/client roles; allow service_role only.
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

-- RPC execute: service_role only (not anon/authenticated PostgREST clients).
revoke all on function public.dispatch_create_prepared(text, text, jsonb, jsonb) from public, anon, authenticated;
revoke all on function public.dispatch_cas_advance(text, text, bigint, text, text) from public, anon, authenticated;
revoke all on function public.dispatch_request_run_stop(text, bigint, text) from public, anon, authenticated;
revoke all on function public.dispatch_get_run_protocol_version(text) from public, anon, authenticated;

grant execute on function public.dispatch_create_prepared(text, text, jsonb, jsonb) to service_role;
grant execute on function public.dispatch_cas_advance(text, text, bigint, text, text) to service_role;
grant execute on function public.dispatch_request_run_stop(text, bigint, text) to service_role;
grant execute on function public.dispatch_get_run_protocol_version(text) to service_role;
