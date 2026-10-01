-- CA-1082: allow authenticated users to update the OVERRIDE columns on
-- built-in workflow rows, while keeping every other column locked.
--
-- Background: BUG-NOTE-CP42 #20 (20260701100000) restricted all
-- authenticated writes on workflows to `is_builtin = false` rows, so direct
-- client calls cannot mutate mirrored built-in definitions. But the app
-- contract (CP-55 / SupabaseAdminRepository.saveWorkflow) deliberately allows
-- editing the override fields — model_override, reasoning_effort_override,
-- yolo_mode — on built-in workflows from the Settings page. Under the
-- `authenticated` role that UPDATE matched zero rows, and the repository's
-- `.select("*").single()` turned it into PGRST116 "Cannot coerce the result
-- to a single JSON object".
--
-- A policy's using/with check sees row values, not column diffs, so the
-- column whitelist is enforced by a BEFORE UPDATE trigger: for the
-- `authenticated` role any change outside the override columns raises 42501.
-- Privileged writers bypass — mirror sync authenticates as service_role, and
-- migrations/direct SQL have no PostgREST JWT so auth.role() is '' there.

create or replace function public.enforce_builtin_workflow_override_columns()
returns trigger
language plpgsql
as $$
begin
  if coalesce(auth.role(), '') <> 'authenticated' then
    return new;
  end if;
  if not (old.is_builtin or coalesce(old.editable, true) = false) then
    return new;
  end if;
  if (to_jsonb(old)
        - 'model_override' - 'reasoning_effort_override' - 'yolo_mode' - 'updated_at')
     is distinct from
     (to_jsonb(new)
        - 'model_override' - 'reasoning_effort_override' - 'yolo_mode' - 'updated_at')
  then
    raise exception
      'built-in workflows only allow override-column updates (model_override, reasoning_effort_override, yolo_mode)'
      using errcode = '42501';
  end if;
  return new;
end;
$$;

drop trigger if exists workflows_builtin_override_columns_guard on public.workflows;
create trigger workflows_builtin_override_columns_guard
  before update on public.workflows
  for each row
  execute function public.enforce_builtin_workflow_override_columns();

-- Permits the UPDATE statement itself for built-in rows; the trigger above
-- rejects anything beyond the override columns.
create policy "authenticated update builtin workflow overrides"
  on workflows for update to authenticated
  using (is_builtin = true)
  with check (is_builtin = true);
