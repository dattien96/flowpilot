# CA-247: Reclaim Or Retire Stale Builtin Flow Mirrors

## What changed

- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`: added `ReclaimOrRetireStaleBuiltinMirrors` — for a pack's `is_builtin=true` rows whose `pack_flow_id` no longer matches any flow the pack currently declares, either patches the row's `pack_flow_id` back in place (by row `id`, when its `pack_hash` still matches a current flow and that flow's slot isn't already occupied) or retires it (`is_builtin=false`, `editable=true`, name suffixed) when it can't be reclaimed. Added the optional `builtinStaleMirrorReclaimer` interface, `flowHashID`, `dbWorkflowIdentityRow`, `staleMirrorSuffix`, and `patchWorkflowByID` helper.
- `apps/local-runner/internal/runner/flow_definition_resolver.go`: `FlowMirrorSyncService.SyncBuiltins` now type-asserts the store for `builtinStaleMirrorReclaimer` and invokes it once per pack, before the existing per-flow upsert loop.
- Added tests: `TestReclaimOrRetireStaleBuiltinMirrorsReclaimsByHashWhenSlotIsFree`, `TestReclaimOrRetireStaleBuiltinMirrorsRetiresWhenSlotAlreadyOccupied`, `TestReclaimOrRetireStaleBuiltinMirrorsRetiresWhenNoHashMatch`, `TestReclaimOrRetireStaleBuiltinMirrorsSkipsAlreadyRetiredRows` (`supabase_workflow_flow_store_test.go`); `TestFlowMirrorSyncInvokesStaleMirrorReclaimerBeforeUpserting`, `TestFlowMirrorSyncWorksWithoutStaleMirrorReclaimerSupport` (`flow_definition_resolver_test.go`).

## Why

Found live: manually corrupting a builtin mirror's `pack_flow_id` and restarting the runner did trigger the documented self-heal (CP-36 Scenario 14), but left the old row behind as a permanent UI-visible duplicate and dropped the row's `model_override` to the raw column default on the fresh insert. See [BUG-249](../requirements/09-BugFix/done/BUG-249-Corrupted-Builtin-Mirror-Recreate-Duplicates-Row-And-Drops-Overrides.md).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-249
change_type: bugfix
summary: reclaim a corrupted builtin mirror row in place by content hash, or retire it, instead of leaving a duplicate on sync
# --->8---
