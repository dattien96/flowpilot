# CA-1075: flow mirror sync must survive unmigrated remotes and aborted step swaps

Date: 2026-10-01
Refs: CA-1072 (`vibe-tasks` builtin), BUG-474 (definition_json snapshot +
degradation contract), CP-42 (mirror into `workflows`/`workflow_steps`),
live diagnostic run of `SyncBuiltins` against the production Supabase
project.

## Problem

`vibe-tasks` never appeared in the desktop Flow picker or the Settings
workflows list even though the shipped binary embedded
`flow-pack/flows/vibe-tasks.yaml`. The desktop reads the remote
`workflows` table directly, so a builtin flow is only visible after
`EnsureBuiltinFlowMirrorsWithStore` (runner boot) writes its mirror row —
and that sync was silently aborting on the **first** flow every boot.

Two independent defects, both in `supabase_workflow_flow_store.go`:

1. **`PGRST204` not recognized.** The remote project has never run
   `supabase/migrations/20260922000000_add_workflow_definition_json.sql`,
   so every upsert payload containing `definition_json` is rejected by
   PostgREST's schema cache with
   `{"code":"PGRST204","message":"Could not find the 'definition_json' column of 'workflows' in the schema cache"}`.
   `isUndefinedDefinitionJSONColumn` only matched SQLSTATE `42703`
   (Postgres undefined-column, emitted when the statement actually
   executes). PGRST204 is raised before the query runs, so the BUG-474
   strip-and-retry fallback never fired → every upsert hard-failed →
   `SyncBuiltins` aborted on `review-loop` (the first manifest flow).
   Every builtin added since the remote schema drifted — `cp-harness*`,
   `bug-plan-harness`, `vibe-*` — had no mirror row.

2. **Stale offset-row collision.** Once upserts could pass, step
   replacement failed on `bug-plan-harness` with `23505` on
   `unique(workflow_id, order_index)` at `order_index=1000006` — an
   orphaned row left in the offset band by a `replaceSteps` that died
   between insert and delete-superseded (the code's own error text
   already promises "retry the save to clean up" — but the retry
   collided on the same key before ever reaching cleanup, so no retry
   could converge).

## Solution

- `isUndefinedDefinitionJSONColumn` now accepts `e.Code == "PGRST204"`
  alongside `42703`, still gated on status 400 + message containing
  `definition_json`. On match the store caches
  `noDefinitionJSONColumn`, strips the key, and retries — the existing
  degradation contract, now covering the error shape the schema cache
  actually emits.
- `replaceSteps` retries lazily: when the offset insert surfaces a
  `23505` unique violation (`isUniqueViolationErr` — inserts only ever
  write the offset band, so 23505 there can only mean orphan rows), it
  calls `deleteOffsetSteps` (DELETE
  `workflow_steps?workflow_id=eq.X&order_index=gte.1000000`) and retries
  the insert once. No completed save can legitimately hold
  `order_index >= insertOrderIndexOffset` — `renormalizeOrderIndex`
  always restores `0..N-1` — so offset-band rows are orphans by
  construction. Lazy (not upfront) keeps the common path byte-for-byte
  identical, so the BUG-NOTE-CP42 #18 ordering tests
  (`...ReplaceStepsInsertsBeforeDeleting`,
  `...InsertFailureLeavesNoDeleteCall`) pass unmodified.

## Files

- `internal/runner/supabase_workflow_flow_store.go` — PGRST204 arm in
  `isUndefinedDefinitionJSONColumn`; `isUniqueViolationErr`;
  `deleteOffsetSteps` + lazy-retry call site in `replaceSteps`;
  `strconv` import.
- `internal/runner/bug474_cloned_flow_execution_fields_test.go` —
  `TestCA1075_UpsertDegradesOnPGRST204SchemaCacheMiss` (the live-observed
  error body verbatim; was red before the fix),
  `TestCA1075_ReplaceStepsClearsStaleOffsetOrphans` (insert → 23505 →
  orphan-delete → retry → superseded-delete), and
  `TestCA1075_ReplaceStepsNonUniqueInsertFailurePropagates` (non-23505
  insert failures never trigger cleanup/retry).

## Verification

- `TestCA1075_UpsertDegradesOnPGRST204SchemaCacheMiss` failed pre-fix
  with the exact live 400 body; passes now, asserting first POST keeps
  `definition_json` and the retry strips it.
- Live `SyncBuiltins` probe (real workspace config + service-role
  secret): **all 8 builtin flows synced, including `vibe-tasks`** —
  `flowpilot-core-flow-pack/vibe-tasks` mirror row created. Previously
  the same call died on the first flow.
- `TestBUG474_*` unchanged and green; `go vet ./internal/runner/` clean.
- Probe test `zz_mirror_probe_test.go` removed after verification
  (diagnostic only, hard-codes the real project).

## Residual

The remote still lacks the `definition_json` migration — snapshots are
skipped (`noDefinitionJSONColumn` cached) and user-clone backfill heals
via builtin source as designed. Applying
`20260922000000_add_workflow_definition_json.sql` to the remote restores
full persistence; the store auto-resumes snapshot writes on next boot.
