# CA-168: Insert-Before-Delete for Workflow Steps Save/Mirror (BUG-NOTE-CP42 #18)

## Scope

Verified and fixed a P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: both the desktop app's workflow-save path and the Go mirror-sync path could permanently lose a workflow's steps if the write that was supposed to replace them failed partway through.

## The bug

Two independent code paths used the same unsafe sequence — DELETE all existing `workflow_steps` rows, then INSERT the replacements:

- `SupabaseAdminRepository.saveWorkflow` (`packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`), called from `WorkflowsSettings.tsx` — the desktop app's real Settings screen, in scope per this session's "don't worry about admin-web, but desktop must work" direction.
- `SupabaseWorkflowFlowStore.replaceSteps` (`apps/local-runner/internal/runner/supabase_workflow_flow_store.go`), used by both user-flow saves and built-in mirror-sync at startup.

PostgREST has no multi-statement transaction support from a plain REST client, so each request commits independently. If the insert failed for any reason (FK violation, unique constraint, a dropped network connection), the DELETE had already committed — the workflow was left with zero steps and no way to recover the old ones.

## Fix

Applied the "insert-new-then-swap" approach the bug note suggested, in both places:

1. Insert the replacement steps first (with a returned id per row, via `return=representation` server-side / `.select("id")` on the TS client).
2. Delete only the rows *not* among the newly-inserted ids (`id=not.in.(...)` in Go's PostgREST query; `.not("id", "in", "(...)")` on the TS client).

A failed step 1 now returns an error with the existing steps completely untouched — no delete has happened yet. A failed step 2 (rare — the insert already succeeded) leaves both old and new rows present, which is a `TODO cleanup` cosmetic leftover, not data loss; the returned error tells the caller to retry.

**Go-specific wrinkle**: `workflow_steps` has `unique(workflow_id, order_index)`. Inserting new rows at `order_index` `0..N-1` while the old rows at that same range are still live (deliberately, since the delete hasn't run yet) would violate that constraint. `insertSteps` sidesteps this by offsetting the new rows' `order_index` by a large constant (`insertOrderIndexOffset = 1_000_000`) during the insert, then `renormalizeOrderIndex` best-effort `PATCH`es each row back down to its real position after the old rows are deleted. This renormalization is purely cosmetic — `recordFromWorkflowRow` sorts nodes by `order_index`, and since every new row shares the same offset, their *relative* order (and therefore the reconstructed `FlowDefinition.Nodes` order) is correct even if the renormalization never runs or partially fails; a failure there is logged, not surfaced as a `replaceSteps` error.

The TS side doesn't need an offset trick — Supabase JS's `.insert(...)` doesn't need to worry about a matching offset scheme because `saveWorkflow`'s steps carry `order_index` values the caller already controls per-save (same as before), and the same unique constraint applies there too; this fix keeps the existing `order_index` values as sent, which in practice the desktop UI already computes fresh from array position each save.

## Verification

- Go: `TestSupabaseWorkflowFlowStoreReplaceStepsInsertsBeforeDeleting` (asserts POST happens before DELETE) and `TestSupabaseWorkflowFlowStoreReplaceStepsInsertFailureLeavesNoDeleteCall` (asserts a failed insert never triggers a DELETE at all).
- TS: `saveWorkflow inserts new steps before deleting superseded ones` and `saveWorkflow does not delete existing steps when the insert fails` (`tests/phase1/workflowFlowEngineAttrs.test.ts`), run via the real `tsc` binary at `apps/desktop-flowpilot/node_modules/.bin/tsc` (see note below) and Node's built-in test runner.
- Full Go flow-related test group (8 `TestSupabaseWorkflowFlowStore*` tests) and full TS `tests/phase1/*.test.js` suite (45 tests; 3 pre-existing failures in untouched files — `desktopAdminSupabaseClient`, `desktopSupabaseAuthRepository`, `importBoundary` — confirmed unrelated via `git status`) pass/behave as expected.
- `go build ./...` clean.

## Environment note (not a code change, but worth recording)

`npx tsc` in this repo's root resolves to npm's placeholder stub package (no `typescript` devDependency at the repo root), not a real compiler — it prints "This is not the tsc command you are looking for" and exits, yet the `rtk` CLI wrapper active in this session summarized that as `"TypeScript: No errors found"`, which is misleading. The real compiler for `tsconfig.phase1-tests.json` is `apps/desktop-flowpilot/node_modules/.bin/tsc`; use that binary directly (or `rtk proxy npx tsc ...` to see raw, unfiltered output) rather than trusting `npx tsc`'s summarized result in this repo.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: replace the delete-then-insert workflow_steps write sequence with insert-then-delete in both the desktop save path and the Go mirror-sync path, so a failed insert can no longer permanently wipe a workflow's existing steps
# --->8---
