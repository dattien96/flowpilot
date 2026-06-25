# BUG-135: Create Project Fails With legacy_id Not-Null Constraint

## Metadata

- Document ID: `BUG-135`
- Title: `Create Project Fails With legacy_id Not-Null Constraint`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-04: Project Management](../../07-Coding-Plan/done/CP-04-Project-Management.md)
- Child Documents: `None`
- Related Documents: [CA-002: Projects UUID Baseline](../../../change-audit/CA-002-projects-uuid-baseline.md), [BUG-108: Project Settings Edit Fails With legacy_project_id Not-Null Constraint](./BUG-108-Project-Settings-Edit-Fails-With-legacy-project-id-Not-Null-Constraint.md)
- Replaces: `None`
- Tags: `project, supabase, migration, regression`

## AI Quick View

### Summary

- Creating a new project fails with: `null value in column "legacy_id" of relation "projects" violates not-null constraint`.
- Root cause: `createProject` in `supabaseAdminRepository.ts` inserts a new project row without supplying `legacy_id`. After migration `20260519070000_projects_uuid_baseline`, `projects.legacy_id` is NOT NULL with a unique constraint and has no column default.
- The correct pattern already exists in `supabase-gateway-bundle.ts/createProject`, which generates a `project_<18hex>` text ID and passes it as `legacy_id`.

### Current Ask

- Generate a `legacy_id` value in `supabaseAdminRepository.ts:createProject` and include it in the insert payload.

### Key Decisions

- `V-1` Generate `legacy_id` as `` `project_${crypto.randomUUID().replaceAll("-", "").slice(0, 18)}` `` — the same pattern used by `supabase-gateway-bundle.ts`.
- `V-2` Both the TS source (`packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`) and its pre-compiled JS copy (`.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js`) are updated.

### Constraints

- No schema changes required; fix is application-side only.
- The `id` column already has a `gen_random_uuid()` default — only `legacy_id` needs to be supplied explicitly.

### Open Questions

- None.

### Source Refs

- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` — `createProject` (line ~252)
- `.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js` — JS copy
- `supabase/migrations/20260519070000_projects_uuid_baseline.sql` — renames `id` → `legacy_id`, adds NOT NULL
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts` — `createProject` (correct reference pattern at line ~601)

## 1. Issue Summary

When a user creates a new project (via the Desktop Settings page or any caller using `supabaseAdminRepository.createProject`), the operation fails with:

```
null value in column "legacy_id" of relation "projects" violates not-null constraint
```

`createProject` in `supabaseAdminRepository.ts` inserts a row into `projects` without including `legacy_id`. After migration `20260519070000_projects_uuid_baseline`, the old text `id` column was renamed to `legacy_id` and retained as NOT NULL (no default). Any insert that omits it is rejected by Postgres.

The companion implementation in `supabase-gateway-bundle.ts` already handles this correctly by generating `legacy_id` with `createId("project")` before the insert.

## 2. Parent Links

- coding plan: [CP-04: Project Management](../../07-Coding-Plan/done/CP-04-Project-Management.md)
- tech design: none (schema change documented in change-audit)
- system spec: none

## 3. Environment and Reproduction

- environment: Desktop app, Settings → Projects → Create Project; also any direct caller of `supabaseAdminRepository.createProject`
- reproduction steps:
  1. Open Settings → Projects.
  2. Fill in project name, description, platform.
  3. Click "Create Project".
  4. Observe the Postgres NOT NULL constraint error.
- frequency: Consistent — every project creation via `supabaseAdminRepository.createProject` triggers this

## 4. Expected vs Actual

- expected: Project is created successfully with a generated `legacy_id` value like `project_a1b2c3d4e5f6a1b2c3`.
- actual: Insert into `projects` fails with `null value in column "legacy_id"` because the column is omitted from the payload.

## 5. Impact

- users affected: All users attempting to create a new project via the Desktop Settings page
- workflows affected: Project creation flow — completely broken for the desktop path using `supabaseAdminRepository`
- severity: High — new project creation is fully blocked

## 6. Root Cause

- hypothesis: `createProject` in `supabaseAdminRepository.ts` was written (or last updated) before the UUID baseline migration and was never updated to supply `legacy_id`.
- confirmed cause:
  ```typescript
  // BEFORE (broken after migration 20260519070000):
  const { data, error } = await this.supabase.from("projects").insert({
    name: input.name,
    // ... other fields ...
    // legacy_id missing — NOT NULL, no default → constraint violation
  }).select("*").single();
  ```
  Migration `20260519070000_projects_uuid_baseline.sql` renamed the old text primary key `id` to `legacy_id`, set NOT NULL, and added a unique constraint — but no default. The insert path in `supabaseAdminRepository.ts` was not updated to generate and supply this value.
- evidence: Error message names `legacy_id` on `projects`; code inspection confirms the field is absent from the insert; `supabase-gateway-bundle.ts:createProject` already generates and includes `legacy_id` correctly.

## 7. Fix Strategy

- `F-1` In `supabaseAdminRepository.ts:createProject`, generate a legacy ID before the insert:
  ```typescript
  const legacyId = `project_${crypto.randomUUID().replaceAll("-", "").slice(0, 18)}`;
  ```
- `F-2` Include `legacy_id: legacyId` in the insert payload:
  ```typescript
  await this.supabase.from("projects").insert({
    legacy_id: legacyId,
    name: input.name,
    // ...
  })
  ```
- `F-3` Apply the same fix to the pre-compiled JS copy in `.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js`.

## 8. Validation

- `V-1` `npx tsc --noEmit` on `packages/flowpilot-client-core` passes with no errors after the fix.
- `V-2` Manual reproduction: creating a project in Settings no longer returns the constraint error (requires a live Supabase instance — not verifiable in CI without one).
- `V-3` Pattern parity with `supabase-gateway-bundle.ts:createProject` confirmed — both now generate and supply `legacy_id` when inserting into `projects`.

## 9. Regression Guard

- tests: No existing unit test covers `supabaseAdminRepository.createProject` with the `legacy_id` field. The correct pattern is covered in `supabase-gateway-bundle.test.ts` for the admin-web path.
- alerts: None.
- audit checks: Any future insert into `projects` via `supabaseAdminRepository` must include `legacy_id` until the column is dropped (planned after slug → UUID migration fully completes).

## 10. Follow-Up Document Updates

- upstream docs that must change: None — the migration contract is already documented in `CA-002-projects-uuid-baseline.md`.
- notes left unchanged on purpose: The `id` column insert is not needed — it has a `gen_random_uuid()` default. Only `legacy_id` required explicit generation.
