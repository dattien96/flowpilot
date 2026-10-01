# CA-1082: builtin workflow override saves blocked by RLS (PGRST116)

Date: 2026-10-01
Refs: user-reported — "Cannot coerce the result to a single JSON object" when
saving a Model override on the workflow page (builtin flows like Vibe Tasks);
step-definition saves worked fine.

## Root cause

`SupabaseAdminRepository.saveWorkflow` intentionally supports an
override-only save on built-in workflows (`editable === false` → UPDATE only
`model_override`/`reasoning_effort_override`/`yolo_mode`).

But migration `20260701100000` (BUG-NOTE-CP42 #20) restricted **all**
`authenticated`-role writes on `workflows` to `is_builtin = false` rows —
including the sanctioned override path. When the desktop's admin client rides
a user session (authenticated role), the UPDATE matches 0 rows and the
repository's `.select("*").single()` surfaces PostgREST PGRST116 "Cannot
coerce the result to a single JSON object".

Verified live on the remote:

- `PATCH /rest/v1/workflows?id=eq.<vibe-tasks>` with a **user JWT** → `[]`
  (RLS-filtered update).
- Same PATCH with **service_role** → row returned (SRK bypasses RLS).
- `step_definitions` worked because its write policy is `using (true)` —
  hence "save step OK, save workflow fails".

## Fix

`supabase/migrations/20261001110000_builtin_workflow_override_columns.sql`:

- New UPDATE policy `authenticated update builtin workflow overrides`
  (`using/with check: is_builtin = true`) permits the statement on builtin
  rows.
- New `BEFORE UPDATE` trigger `enforce_builtin_workflow_override_columns`
  enforces the column whitelist — any change outside
  `model_override`/`reasoning_effort_override`/`yolo_mode`/`updated_at` on a
  builtin row raises `42501`. Whitelist is implemented as a `jsonb` key-minus
  diff so future columns are protected automatically.
- Trigger only fires for `auth.role() = 'authenticated'` — service_role
  (mirror sync) and non-PostgREST contexts (migrations, direct SQL) bypass.

No client change: the repository's existing `.update().select("*").single()`
now returns the row under authenticated, and unmodified-column mutations of
builtin rows stay blocked — BUG-NOTE-CP42 #20's invariant is preserved.

## Files

- `supabase/migrations/20261001110000_builtin_workflow_override_columns.sql`

## Verification

- Live-reproduced the mechanism pre-fix (authenticated PATCH → `[]`;
  service-role PATCH → row).
- Post-apply: replay the same authenticated PATCH — must return the row;
  a `name` change under authenticated must raise the trigger error.
- Apply path: Settings → Supabase setup → apply migrations (needs the
  management API token), or `supabase db push`.

## Note

When the desktop's `serviceRoleKey` is configured, the admin client uses it
and bypasses RLS entirely — restarting the app also unblocks the save
without this migration. The migration closes the gap for authenticated-only
sessions and removes the cryptic PGRST116 surface.
