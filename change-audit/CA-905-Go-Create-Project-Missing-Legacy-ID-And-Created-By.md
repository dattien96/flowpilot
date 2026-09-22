# CA-905: Go Create Project Missing legacy_id And created_by

## Summary

- Bug (live repro from TUI onboard wizard): `POST /client/projects` →
  PostgREST 400 `23502: null value in column "legacy_id" of relation
  "projects" violates not-null constraint`, surfaced as
  `runner API error 500 (create_project_failed)`.
- Root cause: `SupabaseCatalogStore.CreateProject` never sent `legacy_id`
  or `created_by`. The same defect was already fixed on the TS paths —
  BUG-135 added `legacy_id` (`project_<18hex>`) and BUG-136 added
  `created_by` ("supabase-admin") to `supabaseAdminRepository.ts`; the Go
  port of the insert was never updated. `created_by` would have been the
  next 23502 after `legacy_id`, so both are supplied now.
- Fix: insert payload now includes `legacy_id` via `newProjectLegacyID()`
  (crypto/rand, `project_` + 18 hex chars — same shape as the TS pattern)
  and `created_by: "supabase-admin"`.

## Files

- `apps/local-runner/internal/runner/supabase_catalog_store.go` (payload +
  `newProjectLegacyID` helper)
- `apps/local-runner/internal/runner/supabase_catalog_create_test.go`
  (new `TestCreateProjectSendsLegacyIDAndCreatedBy` — red before, green after)

## Out of Scope

- `TestCreateProject_AutoTriggersScaffoldForCapablePlatform` fails in this
  working tree with and without this change (verified via targeted stash):
  broken by the in-progress CP-71 run-worktree edits in the same package,
  not by this fix.
- A DB-side default for `legacy_id`/`created_by` would let every insert
  path drop these fields — a migration decision, not made here.

# ---8<--- flowpilot:change-ledger
feature_key: supabase-config
source_doc_id: live-repro-tui-onboard
change_type: bugfix
summary: supply legacy_id + created_by in Go SupabaseCatalogStore.CreateProject insert
# --->8---
