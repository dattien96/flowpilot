# BUG-136: Create Project Fails With created_by Not-Null Constraint

## Metadata

- Document ID: `BUG-136`
- Title: `Create Project Fails With created_by Not-Null Constraint`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-04: Project Management](../../07-Coding-Plan/done/CP-04-Project-Management.md)
- Child Documents: `None`
- Related Documents: [BUG-135: Create Project Fails With legacy_id Not-Null Constraint](./BUG-135-Create-Project-Fails-With-legacy-id-Not-Null-Constraint.md), [CA-124: Fix Create Project created_by Not-Null Constraint](../../../change-audit/CA-124-fix-create-project-created-by-not-null-constraint.md)
- Replaces: `None`
- Tags: `project, supabase, regression, constraint`

## AI Quick View

### Summary

- Creating a new project fails with: `null value in column "created_by" of relation "projects" violates not-null constraint`.
- Root cause: `createProject` in `supabaseAdminRepository.ts` inserts a new project row without supplying `created_by`. The `projects.created_by` column is `text not null` per migration `20260515050000_admin_mvp_skeleton.sql` and has no column default.
- The correct pattern already exists in `supabase-gateway-bundle.ts:createProject`, which passes `created_by: "supabase-admin"` in the insert payload.
- This is a sibling defect to BUG-135 (missing `legacy_id`), which was fixed in the same file but did not add `created_by` at that time.

### Current Ask

- Add `created_by: "supabase-admin"` to the insert payload in `supabaseAdminRepository.ts:createProject` and its pre-compiled JS copy.

### Key Decisions

- `V-1` Use `"supabase-admin"` as the `created_by` value — the same literal used by `supabase-gateway-bundle.ts:createProject` (line 621).
- `V-2` Both the TS source (`packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`) and its pre-compiled JS copy (`.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js`) are updated.

### Constraints

- No schema changes required; fix is application-side only.
- `created_by` has no default in the schema — it must be supplied explicitly on every insert.

### Open Questions

- None.

### Source Refs

- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` — `createProject` (line ~254)
- `.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js` — `createProject` (line ~202)
- `supabase/migrations/20260515050000_admin_mvp_skeleton.sql` — defines `created_by text not null` on `projects`
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts` — `createProject` (correct reference at line ~621)

## 1. Issue Summary

When a user creates a new project (via the Desktop Settings page or any caller using `supabaseAdminRepository.createProject`), the operation fails with:

```
null value in column "created_by" of relation "projects" violates not-null constraint
```

`createProject` in `supabaseAdminRepository.ts` inserts a row into `projects` without including `created_by`. The original schema (`20260515050000_admin_mvp_skeleton.sql`) declares `created_by text not null` with no column default, so any insert that omits it is rejected by Postgres.

The companion implementation in `supabase-gateway-bundle.ts` already handles this correctly by passing `created_by: "supabase-admin"` in its insert payload. The BUG-135 fix for the same file added `legacy_id` but did not also add `created_by`.

## 2. Parent Links

- coding plan: [CP-04: Project Management](../../07-Coding-Plan/done/CP-04-Project-Management.md)
- tech design: none
- system spec: none

## 3. Environment and Reproduction

- environment: Desktop app, Settings → Projects → Create Project; also any direct caller of `supabaseAdminRepository.createProject`
- reproduction steps:
  1. Open Settings → Projects.
  2. Fill in project name, description, platform.
  3. Click "Create Project".
  4. Observe the Postgres NOT NULL constraint error on `created_by`.
- frequency: Consistent — every project creation via `supabaseAdminRepository.createProject` triggers this

## 4. Expected vs Actual

- expected: Project is created successfully with `created_by = "supabase-admin"`.
- actual: Insert into `projects` fails with `null value in column "created_by"` because the column is omitted from the payload.

## 5. Impact

- users affected: All users attempting to create a new project via the Desktop Settings page
- workflows affected: Project creation flow — completely blocked for the desktop path using `supabaseAdminRepository`
- severity: High — new project creation is fully blocked

## 6. Root Cause

- hypothesis: `createProject` in `supabaseAdminRepository.ts` was not updated to supply `created_by` when BUG-135 was fixed (only `legacy_id` was added at that time).
- confirmed cause:
  ```typescript
  // BEFORE (broken — created_by omitted):
  const { data, error } = await this.supabase.from("projects").insert({
    legacy_id: legacyId,
    name: input.name,
    // ... other fields ...
    // created_by missing — NOT NULL, no default → constraint violation
  }).select("*").single();
  ```
  Migration `20260515050000_admin_mvp_skeleton.sql` declares `created_by text not null` on the `projects` table. No subsequent migration adds a default. BUG-135 added `legacy_id` to this insert but omitted `created_by`.
- evidence: Error message names `created_by` on `projects`; code inspection confirms the field is absent from the insert in `supabaseAdminRepository.ts`; `supabase-gateway-bundle.ts:createProject` already passes `created_by: "supabase-admin"` correctly.

## 7. Fix Strategy

- `F-1` In `supabaseAdminRepository.ts:createProject`, add `created_by: "supabase-admin"` to the insert payload.
- `F-2` Apply the identical change to the pre-compiled JS copy in `.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js`.

## 8. Validation

- `V-1` Pattern parity with `supabase-gateway-bundle.ts:createProject` confirmed — both now supply `created_by: "supabase-admin"` when inserting into `projects`.
- `V-2` Manual reproduction: creating a project in Settings no longer returns the constraint error (requires a live Supabase instance — not verifiable in CI without one).
- `V-3` `npx tsc --noEmit` on `packages/flowpilot-client-core` expected to pass — change adds a known required column with a string literal.

## 9. Regression Guard

- tests: No existing unit test covers `supabaseAdminRepository.createProject` with the `created_by` field. Pattern coverage exists in `supabase-gateway-bundle.test.ts` for the admin-web path.
- alerts: None.
- audit checks: Any future insert into `projects` via `supabaseAdminRepository` must include both `legacy_id` and `created_by` until those columns gain defaults or are dropped.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — the `created_by not null` constraint is part of the original schema in `20260515050000_admin_mvp_skeleton.sql`.
- notes left unchanged on purpose: `supabase-gateway-bundle.ts` was already correct and required no changes.
