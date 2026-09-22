# CA-905: Go Create Project Missing NOT NULL Columns

## Summary

- Bug (live repro from TUI onboard wizard): `POST /client/projects` →
  PostgREST 400 `23502: null value in column "legacy_id" of relation
  "projects" violates not-null constraint`, surfaced as
  `runner API error 500 (create_project_failed)`. After `legacy_id` +
  `created_by` landed, the next identical failure surfaced on
  `description` — the table has four NOT NULL columns with no default
  that the Go insert omitted entirely.
- Root cause: `SupabaseCatalogStore.CreateProject` never sent
  `legacy_id`, `created_by`, `description`, or `repository_url`. The same
  defect was already fixed on the TS paths — BUG-135 added `legacy_id`
  (`project_<18hex>`) and BUG-136 added `created_by`
  ("supabase-admin") to `supabaseAdminRepository.ts`; the Go port of the
  insert was never updated.
- Fix: insert payload now includes `legacy_id` via `newProjectLegacyID()`
  (crypto/rand, `project_` + 18 hex chars — same shape as the TS
  pattern), `created_by: "supabase-admin"`, and `description` /
  `repository_url` (new optional `CreateProjectInput` fields; empty
  string satisfies the NOT NULL constraint when unset).
- `default_provider` / `default_reasoning_effort` /
  `session_idle_ttl_minutes` / `artifact_storage_preference` have DB
  defaults and need no payload fields.

## Files

- `apps/local-runner/internal/runner/supabase_catalog_store.go`
  (payload fields + `newProjectLegacyID` helper + input fields)
- `apps/local-runner/internal/runner/supabase_catalog_create_test.go`
  (new `TestCreateProjectSendsLegacyIDAndCreatedBy` asserts all four
  NOT NULL columns — red before, green after)

## Out of Scope

- `TestCreateProject_AutoTriggersScaffoldForCapablePlatform` fails in this
  working tree with and without this change (verified via targeted stash):
  broken by the in-progress CP-71 run-worktree edits in the same package,
  not by this fix.
- DB-side defaults for `legacy_id`/`created_by`/`description`/
  `repository_url` would let every insert path drop these fields — a
  migration decision, not made here.

# ---8<--- flowpilot:change-ledger
feature_key: supabase-config
source_doc_id: live-repro-tui-onboard
change_type: bugfix
summary: supply legacy_id, created_by, description, repository_url in Go CreateProject insert
# --->8---
