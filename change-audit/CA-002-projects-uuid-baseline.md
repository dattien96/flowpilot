# CA-002 Projects UUID Baseline

## Issue

The current Supabase skeleton still defines `projects.id` and all related `project_id` foreign keys as `TEXT`, while the CP-04+ planning docs assume UUID-based project keys.

The mismatch appears in:

- `supabase/migrations/20260515050000_admin_mvp_skeleton.sql`
- `supabase/migrations/20260519060000_cp04_project_team_management.sql`

This blocks later CP migrations from safely creating UUID foreign keys to `projects(id)`.

## Root Cause

The initial admin MVP schema used human-readable text IDs such as:

- `project_meal_suggestion`
- `project_flowpilot_admin`

That was acceptable for the bootstrap seed, but later plans moved to UUID primary keys without a baseline database migration to bridge the old schema to the new one.

## Solution

Add a dedicated baseline migration:

- `supabase/migrations/20260519070000_projects_uuid_baseline.sql`

The migration:

- adds `projects.id_uuid`
- backfills UUID values for all existing projects
- maps dependent `project_id` columns onto the new UUID values
- swaps `projects.id` from text to UUID
- preserves the old text IDs as `legacy_id` / `legacy_project_id` columns for one release

## Scope

The migration converts these project foreign-key paths:

- `features.project_id`
- `context_sources.project_id`
- `workflow_runs.project_id`
- `ai_outputs.project_id`
- `project_teams.project_id`

## Explicit Non-Goals

This migration does **not** convert actor columns such as:

- `projects.created_by`
- `projects.owner_id`
- `features.owner_id`

Those columns still contain legacy text values like `seed` and need a separate `auth.users` mapping migration before they can become UUID foreign keys.

## Verification

After applying the migration, verify:

1. `projects.id` is `UUID`
2. all converted `project_id` columns are `UUID`
3. foreign keys to `projects(id)` are re-created successfully
4. legacy text columns remain available as `legacy_id` / `legacy_project_id`
5. existing seeded project-linked rows still join correctly through the new UUID keys

## Note

This is a compatibility migration, not a full schema cleanup. The legacy text columns are intentionally retained so the app and seed data can transition safely before a later cleanup migration removes them.

# ---8<--- flowpilot:change-ledger
feature_key: supabase-config
source_doc_id: CP-04
change_type: feature
summary: Projects UUID Baseline
# --->8---
