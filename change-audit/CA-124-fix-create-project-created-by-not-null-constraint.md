# CA-124: Fix Create Project created_by Not-Null Constraint

## Scope

Bug fix for `null value in column "created_by" of relation "projects" violates not-null constraint` when creating a new project via the desktop settings path (`supabaseAdminRepository.createProject`).

Files changed:
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` — added `created_by: "supabase-admin"` to the `projects` insert payload in `createProject`
- `.phase1-tests/packages/flowpilot-client-core/src/data/supabaseAdminRepository.js` — identical fix applied to the pre-compiled JS copy

## Completed

- Added `created_by: "supabase-admin"` to the insert object in `supabaseAdminRepository.ts:createProject` (line ~267).
- Applied the same one-line addition to the pre-compiled JS copy in `.phase1-tests/.../supabaseAdminRepository.js` (line ~214).
- No schema changes; `projects.created_by` is `text not null` per the original migration (`20260515050000_admin_mvp_skeleton.sql`) and the fix is application-side only.

## Verification

- Code inspection: `supabase-gateway-bundle.ts:createProject` (line 621) uses the same `"supabase-admin"` literal — parity confirmed.
- Manual end-to-end test against a live Supabase instance not run in this session; constraint error is structurally resolved by supplying the required column.

## Residual Notes

- `supabaseAdminRepository.ts:createProject` now requires both `legacy_id` (added in BUG-135 / CA-002) and `created_by` (this fix) to be explicit. Neither column has a DB-level default.
- A future migration that drops `legacy_id` or adds defaults to `created_by` would allow these to be removed from the insert.
- Tracked in: [BUG-136](../requirements/09-BugFix/done/BUG-136-Create-Project-Fails-With-created-by-Not-Null-Constraint.md)
