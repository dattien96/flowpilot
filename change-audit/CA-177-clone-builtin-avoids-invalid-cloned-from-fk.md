# CA-177: CloneBuiltin Avoids an Invalid cloned_from FK Value (BUG-NOTE-CP42 #12)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `FlowDefinitionResolver.CloneBuiltin` could write a non-UUID value into a `uuid` foreign-key column.

## The bug

The migration's `workflows.cloned_from` column is `uuid references workflows(id) on delete set null`. `CloneBuiltin` set `ClonedFrom: source.FlowRef` unconditionally — but for a built-in source, `FlowRef` is always the stable `"packId/flowId"` string (e.g. `flowpilot-core-flow-pack/review-loop`), never a UUID. `recordFromWorkflowRow` deliberately normalizes a mirrored built-in row's `FlowRef` back to this stable ref (so a re-created mirror row never changes the flow's identity) — there is no way to recover the underlying row's actual UUID through the `FlowDefinitionStore` interface as it exists today. Writing the stable ref into a `uuid`-typed column would fail against a real Postgres database.

This is currently a dead-but-reachable code path: the live desktop UI's own clone flow (`SupabaseAdminRepository.cloneWorkflow`, TypeScript) avoids the problem entirely by using the source row's real `id` directly — it never goes through this Go `CloneBuiltin` API. But the Go path is still part of the public `FlowDefinitionResolver` surface and would break if ever invoked.

## Fix

`CloneBuiltin` now only sets `ClonedFrom` when `source.FlowRef` actually `looksLikeUUID` (reusing the existing helper from `supabase_workflow_flow_store.go`). A built-in source leaves `ClonedFrom` empty — `cloned_from` is nullable, so this is valid, not an error condition. This is a known limitation rather than a full fix: a built-in-sourced clone has no `cloned_from` provenance recorded at all today, since there's genuinely no valid value to put there without extending `FlowDefinitionStore`'s interface to expose raw row UUIDs (out of scope for this fix).

## Verification

- Updated the pre-existing `TestCloneBuiltinCreatesEditableUserCopy`, which had encoded the old (broken) expectation that `ClonedFrom` equals the stable pack ref — it now asserts `ClonedFrom == ""` for a built-in source, with a comment explaining why.
- New test `TestCloneBuiltinFromUUIDSourcePreservesClonedFrom`: seeds the fake store directly with a UUID-shaped `FlowRef` (the shape a user-owned/already-cloned flow has, even though the production store never actually returns this shape from a built-in lookup) to prove the `looksLikeUUID` branch itself correctly preserves `ClonedFrom` when the value is valid.
- Full suite: 1008 passed, 15 pre-existing/environmental failures (unchanged from before this fix).
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: CloneBuiltin only sets cloned_from when the source FlowRef is a real UUID, avoiding an FK-violating write of a built-in's stable "packId/flowId" ref into the uuid-typed cloned_from column
# --->8---
