# CA-188: Recreate a Missing Built-In Mirror at Resolve Time (BUG-NOTE-CP42 #15)

## Scope

Verified and fixed the last P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: selecting a built-in flow whose Supabase mirror row is missing never recreated it — the fallback to the embedded pack was permanent until the next process restart's best-effort startup sync.

## The bug

`FlowDefinitionResolver.ResolveBuiltin` falls back to the embedded pack whenever `store.GetByPackFlow` finds no row (mirror missing — deleted, or never synced because the store wasn't configured yet at the time `EnsureBuiltinFlowMirrorsWithStore` ran at startup). CP-42 requires selecting a built-in whose mirror is missing to trigger a recreate before the run starts, so the mirror stays available for other paths that read from Supabase directly (e.g. `WorkflowsSettings.tsx`, which lists workflows via `SupabaseAdminRepository`, not through this resolver). Without a recreate, deleting a mirror row (accidentally or otherwise) permanently hid that built-in from Supabase-backed views until the app happened to restart.

## Fix

`ResolveBuiltin` now attempts `store.Upsert` with the embedded-pack-derived record (`Source: "supabase_builtin_mirror"`) immediately after computing it, returning the store's saved result on success. If the upsert fails, it falls through to returning the original `builtin_pack`-sourced record — a mirror-sync hiccup must not block a flow the embedded pack alone is sufficient to run.

**Fixed a bug in the fix during implementation**: the first version mutated `record.Source` to `"supabase_builtin_mirror"` *before* attempting the upsert, so even the failure-fallback path returned a record falsely claiming a mirror that was never persisted. Caught by the new `TestFlowDefinitionResolverFallsBackToPackWhenRecreateFails` test itself failing; corrected by building a separate `mirrorAttempt` copy for the upsert call and only ever returning the original record (unmutated `Source`) on the fallback path.

## Verification

- Renamed and rewrote `TestFlowDefinitionResolverFallsBackToPackWhenMirrorMissing` → `TestFlowDefinitionResolverRecreatesMissingMirror` (the old name/assertion encoded the bug itself — asserting `Source == "builtin_pack"` on every missing-mirror resolution, which was the exact behavior CP-42 says is wrong). The new version asserts the record's `Source` reflects the recreated mirror and that a second, independent `GetByPackFlow` call finds the persisted row.
- New test `TestFlowDefinitionResolverFallsBackToPackWhenRecreateFails`: a store wrapper (`failingUpsertFlowDefinitionStore`) whose `Upsert` always errors, asserting resolution still succeeds with `Source == "builtin_pack"`.
- Full suite: 1015 passed (runner), 15 pre-existing/environmental failures unchanged; `internal/agentpack` unaffected.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: ResolveBuiltin now recreates a missing built-in mirror row via store.Upsert before falling back to the embedded pack, so a deleted or never-synced mirror self-heals on next use instead of staying permanently absent from Supabase until the next restart
# --->8---
