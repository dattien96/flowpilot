# BUG-108: Project Settings Edit Fails With legacy_project_id Not-Null Constraint

## Metadata

- Document ID: `BUG-108`
- Title: `Project Settings Edit Fails With legacy_project_id Not-Null Constraint`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-04: Project Management](../../07-Coding-Plan/done/CP-04-Project-Management.md)
- Child Documents: `None`
- Related Documents: [CA-002: Projects UUID Baseline](../../../change-audit/CA-002-projects-uuid-baseline.md)
- Replaces: `None`
- Tags: `settings, project, supabase, migration, regression`

## AI Quick View

### Summary

- Saving a project in the Settings page fails with: `null value in column "legacy_project_id" of relation "project_teams" violates not-null constraint`.
- Root cause: `setProjectTeams` in `supabaseAdminRepository.ts` inserts rows into `project_teams` with only `{ project_id, team_id }`. After migration `20260519070000_projects_uuid_baseline`, `project_teams.legacy_project_id` is NOT NULL and must carry the old text project ID — but `setProjectTeams` never fetches or supplies it.
- The same pattern is already correct in `supabase-gateway-bundle.ts/linkTeamToProject`, which fetches `legacy_id` from `projects` before inserting.

### Current Ask

- Before inserting into `project_teams`, fetch `projects.legacy_id` and include it as `legacy_project_id` in every inserted row.

### Key Decisions

- `V-1` The fetch is a single `.select("legacy_id").eq("id", projectId).single()` — the same lookup used by `linkTeamToProject` in `supabase-gateway-bundle.ts`.
- `V-2` If `legacy_id` is somehow absent (e.g. a project created post-migration with no legacy text id), fall back to `""` — the constraint violation is avoided; data integrity remains a migration concern, not a runtime concern.
- `V-3` Both the TS source (`packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`) and its pre-compiled JS copy (`.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js`) are updated.

### Constraints

- The delete step (`DELETE FROM project_teams WHERE project_id = ?`) is unaffected — it already uses the UUID `project_id` column correctly.
- No schema changes required; fix is application-side only.

### Open Questions

- None.

### Source Refs

- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` — `setProjectTeams` (line ~405)
- `.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js` — JS copy
- `supabase/migrations/20260519070000_projects_uuid_baseline.sql` — defines `legacy_project_id NOT NULL`
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts` — `linkTeamToProject` (correct reference pattern at line ~868)

## 1. Issue Summary

When a user edits a project in the Settings page and saves, the operation fails with:

```
null value in column "legacy_project_id" of relation "project_teams" violates not-null constraint
```

The Settings page calls `admin.teams.setProjectTeams(project.id, selectedTeamIds)` to replace the project's team links. `setProjectTeams` deletes existing rows then re-inserts them — but the insert payload only supplies `project_id` (UUID) and `team_id`. The `legacy_project_id` column (the old text project ID, retained after the UUID baseline migration) is required NOT NULL and is absent from the insert, causing Postgres to reject it.

## 2. Parent Links

- coding plan: [CP-04: Project Management](../../07-Coding-Plan/done/CP-04-Project-Management.md)
- tech design: none (schema change documented in change-audit)
- system spec: none

## 3. Environment and Reproduction

- environment: Desktop app, Settings → Projects → Edit project, any project with at least one team assigned
- reproduction steps:
  1. Open Settings → Projects.
  2. Select any project and click Edit.
  3. Change any field (name, description, or team assignment) and save.
  4. Observe the Postgres NOT NULL constraint error returned to the UI.
- frequency: Consistent — every project save that includes team links triggers the insert

## 4. Expected vs Actual

- expected: Project saves successfully; `project_teams` rows are replaced with correct `project_id` (UUID) and `legacy_project_id` (text).
- actual: Insert into `project_teams` fails with `null value in column "legacy_project_id"` because the column is omitted from the payload.

## 5. Impact

- users affected: All users editing projects in the Settings page when teams are configured
- workflows affected: Project management settings page — project rename, description, and team assignment all fail
- severity: High — the project settings edit flow is completely broken when team links are present

## 6. Root Cause

- hypothesis: `setProjectTeams` was written before the UUID baseline migration and was never updated to supply `legacy_project_id`.
- confirmed cause:
  ```typescript
  // BEFORE (broken after migration 20260519070000):
  const { error } = await this.supabase.from("project_teams").insert(
    teamIds.map((teamId) => ({ project_id: projectId, team_id: teamId }))
  );
  ```
  Migration `20260519070000_projects_uuid_baseline.sql` renamed the old text `project_id` column to `legacy_project_id` and added a new UUID `project_id`. Both columns are NOT NULL. The insert supplies `project_id` (UUID) but omits `legacy_project_id` (text).
- evidence: Error message names `legacy_project_id` on `project_teams`; code inspection confirms the column is absent from the `setProjectTeams` insert; `linkTeamToProject` in `supabase-gateway-bundle.ts` already has the correct pattern with the `legacy_id` fetch.

## 7. Fix Strategy

- `F-1` Before the insert in `setProjectTeams`, fetch `projects.legacy_id` for the given `projectId`:
  ```typescript
  const { data: projectRow, error: projectError } = await this.supabase
    .from("projects")
    .select("legacy_id")
    .eq("id", projectId)
    .single();
  assertNoError(projectError, "Unable to load project for team link.");
  const legacyProjectId: string = (projectRow as Row)?.legacy_id ?? "";
  ```
- `F-2` Include `legacy_project_id: legacyProjectId` in every inserted row:
  ```typescript
  teamIds.map((teamId) => ({ project_id: projectId, legacy_project_id: legacyProjectId, team_id: teamId }))
  ```
- `F-3` Apply the same fix to the pre-compiled JS copy in `.phase1-tests/`.

## 8. Validation

- `V-1` `npx tsc --noEmit` on `packages/flowpilot-client-core` passes with no errors after the fix.
- `V-2` Manual reproduction: editing a project in Settings no longer returns the constraint error (requires a live Supabase instance — not verifiable in CI without one).
- `V-3` Pattern parity with `linkTeamToProject` in `supabase-gateway-bundle.ts` confirmed — both now fetch `legacy_id` before inserting into `project_teams`.

## 9. Regression Guard

- tests: No existing unit test covers `setProjectTeams` with the `legacy_project_id` field. The correct pattern is guarded in `supabase-gateway-bundle.test.ts` (`TestLinkTeamToProjectWritesBothProjectIdAndLegacyProjectId`), but `supabaseAdminRepository.ts` has no equivalent test.
- alerts: None.
- audit checks: Any future insert into `project_teams` must include `legacy_project_id` until the column is dropped (planned for the next release after slug → UUID migration completes).

## 10. Follow-Up Document Updates

- upstream docs that must change: None — the migration contract is already documented in `CA-002-projects-uuid-baseline.md`.
- notes left unchanged on purpose: The delete step in `setProjectTeams` uses `project_id` (UUID) which is correct after the migration; only the insert path was broken.
