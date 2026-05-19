# CA-001 Supabase Table Policies

## Issue

Creating a team from the Admin Web UI failed with a Supabase `forbidden` response even though the frontend used the standard Supabase SDK correctly.

The failing application path was:

- `apps/admin-web/src/routes/_authenticated/teams.tsx`
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`

The insert call itself is straightforward:

```ts
await supabase.from("teams").insert({ id: createUuid(), name }).select("*").single();
```

## Root Cause

The `public.teams` table had Row Level Security enabled, but it had no policies.

Observed database state:

- `teams` exists
- `rowsecurity = true`
- `pg_policies` returned no rows for `public.teams`

With RLS enabled and no matching policy, PostgreSQL denies all table access for non-bypass roles. That means `INSERT`, `SELECT`, `UPDATE`, and `DELETE` are all blocked unless a policy explicitly allows them.

This is why the Supabase SDK call still failed. The SDK was fine, but the database authorization layer rejected the insert.

## Why This Happened

The repository previously had two relevant migration areas:

- `supabase/migrations/20260515050000_admin_mvp_skeleton.sql`
- `supabase/migrations/20260519060000_cp02_project_team_management.sql`

Before consolidation, the `teams`, `project_teams`, and `team_members` policy definitions lived under `apps/admin-web/supabase/migrations`, while the app README documented only the root admin MVP migration as the database setup step.

That makes it likely the target Supabase project has:

- the `teams` table present
- RLS enabled
- but the team-management policies never applied

## Expected Policies

The current codebase expects these policies for `public.teams`:

- `teams_select_all`
- `teams_insert_all`
- `teams_update_all`
- `teams_delete_all`

And similar permissive policies for:

- `public.project_teams`
- `public.team_members`

## Solution

Apply the team-management migration to the Supabase project that the Admin Web app is using:

- `supabase/migrations/20260519060000_cp02_project_team_management.sql`

If applying the full migration is not practical immediately, create the missing `teams` policies manually in Supabase SQL Editor as a short-term fix.

## Verification

After the fix, verify:

1. `public.teams` still has `rowsecurity = true`
2. `pg_policies` returns the expected `teams_*` rows
3. creating a team from the UI succeeds
4. project-team linking and team-member creation also succeed

## Note

This was not a frontend SDK bug and not a missing custom access-token wrapper issue like the Go backend pattern used in `BeMplan`. In `flowpilot`, the immediate failure was caused by missing RLS policies on the Supabase table itself.
