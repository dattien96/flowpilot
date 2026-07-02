-- BUG-NOTE-CP42 #20: "built-in workflows are read-only" was only enforced in
-- application code (FlowDefinitionResolver.UpdateUserFlow,
-- SupabaseAdminRepository.saveWorkflow) — the RLS policies added by
-- 20260522101000_open_workflow_engine_rls_for_admin.sql grant every
-- authenticated user unconditional `using (true) with check (true)` access,
-- so a direct Supabase client call (bypassing the app entirely) could still
-- mutate a mirrored built-in row, which CP-42 explicitly lists as a failure
-- case to prevent.
--
-- Mirror-sync (EnsureBuiltinFlowMirrorsWithStore, which legitimately writes
-- is_builtin=true rows) always authenticates with the service role key as of
-- BUG-NOTE-CP42 #19's fix (FlowDefinitionStoreFor no longer falls back to an
-- anon key) — and Postgres/Supabase's service_role always bypasses RLS
-- entirely, by design. So restricting the `authenticated` role's write
-- access to non-built-in rows only does not block the legitimate mirror-sync
-- path; it only blocks a direct client-side mutation of a built-in row,
-- exactly the gap this bug describes.

-- workflows: split the single "for all" policy so DELETE gets its own
-- `using` clause (a `with check` alone does not gate DELETE).
drop policy if exists "authenticated write workflows" on workflows;

create policy "authenticated insert workflows"
  on workflows for insert to authenticated
  with check (is_builtin = false);

create policy "authenticated update workflows"
  on workflows for update to authenticated
  using (is_builtin = false)
  with check (is_builtin = false);

create policy "authenticated delete workflows"
  on workflows for delete to authenticated
  using (is_builtin = false);

-- workflow_steps: no is_builtin column of its own — inherit the built-in
-- status of the parent workflow via workflow_id.
drop policy if exists "authenticated write workflow_steps" on workflow_steps;

create policy "authenticated insert workflow_steps"
  on workflow_steps for insert to authenticated
  with check (
    exists (
      select 1 from workflows w
      where w.id = workflow_steps.workflow_id and w.is_builtin = false
    )
  );

create policy "authenticated update workflow_steps"
  on workflow_steps for update to authenticated
  using (
    exists (
      select 1 from workflows w
      where w.id = workflow_steps.workflow_id and w.is_builtin = false
    )
  )
  with check (
    exists (
      select 1 from workflows w
      where w.id = workflow_steps.workflow_id and w.is_builtin = false
    )
  );

create policy "authenticated delete workflow_steps"
  on workflow_steps for delete to authenticated
  using (
    exists (
      select 1 from workflows w
      where w.id = workflow_steps.workflow_id and w.is_builtin = false
    )
  );
