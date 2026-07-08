# BUG-249: Corrupted Builtin Mirror Recreate Duplicates Row And Drops Overrides

## Metadata

- Document ID: `BUG-249`
- Title: `Corrupted Builtin Mirror Recreate Duplicates Row And Drops Overrides`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-07`
- Last Updated: `2026-07-07`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 14), [Task-175: Built-In Flow Mirror Sync And Resolver](../../08-Task/done/Task-175-Builtin-Flow-Mirror-Sync-And-Resolver.md)
- Child Documents: `none`
- Related Documents: [CA-150: Flow Definition Resolver And Mirror Sync](../../../change-audit/CA-150-flow-definition-resolver-and-mirror-sync.md), [CA-160: Flow Definitions Migrated To Workflows Table](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md), [CA-247: Reclaim Or Retire Stale Builtin Flow Mirrors](../../../change-audit/CA-247-reclaim-or-retire-stale-builtin-mirrors.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- Replaces: `none`
- Tags: `agent-flow-engine, flow-definition, supabase, builtin-flow, mirror-sync, regression, data-integrity`

## AI Quick View

### Summary

- Found while the owner manually ran CP-36 Scenario 14 ("Missing Built-in Mirror Row Is Recreated On Demand") against a real Supabase project: corrupt the `review-loop` mirror row's `pack_flow_id` (rename it to `review-loop-haha`), restart the runner, and inspect the `workflows` table before/after.
- The startup sync (`EnsureBuiltinFlowMirrorsWithStore` → `FlowMirrorSyncService.SyncBuiltins`) did recreate a correct mirror row, confirming Scenario 14's own 4 checklist items — but two things it does *not* check for both went wrong: (1) the old corrupted row was left in place, so Settings' Definitions list and the Workflow Mode picker both showed two duplicate "Review Loop / BUILT-IN" entries (owner screenshots); (2) the fresh row's `model_override` reverted to the `workflows` table's own Postgres column default (`gpt-5.4`) instead of the row's previous value (`claude-haiku`), since that column is outside the pack schema entirely.
- Root cause: `Upsert`'s builtin path keys off `on_conflict=pack_id,pack_flow_id`. Once the corrupted row's `pack_flow_id` no longer equals any current pack flow id, no row conflicts with that key, so PostgREST performs a plain `INSERT` — a fresh row, not an update of the existing one. Confirmed via two live Supabase `workflows` CSV exports the owner captured before/after the corruption + restart.

### Current Ask

- Stop a corrupted/renamed builtin mirror's identity key from producing a duplicate row on the next sync, and preserve a reclaimed row's own per-installation columns (`model_override` et al.) when the corruption is repairable.

### Key Decisions

- `V-1` Reclaim, don't just detect: when an orphaned `is_builtin=true` row's `pack_hash` still matches one of the pack's current flows, patch only its `pack_flow_id` back to the correct value by the row's own `id` — never re-insert. This is a true `UPDATE`, so every column the sync path doesn't touch (`model_override`, `provider_override`, `reasoning_effort_override`, `yolo_mode`) survives untouched.
- `V-2` Never reclaim into an already-occupied flow slot. If a fresh row already exists for the target `pack_flow_id` (the duplicate has already happened, e.g. the owner's live DB state at report time), reclaiming would violate the `unique(pack_id, pack_flow_id)` index — retire the orphan instead.
- `V-3` Retire, don't delete. An orphan that can't be reclaimed (hash also changed, or the flow was removed from the pack, or its slot is occupied) gets `is_builtin=false`, `editable=true`, and a `(stale mirror...)` name suffix — never a hard delete, so any clone's `cloned_from` FK (Scenario 14 checklist item 3) is never put at risk.
- `V-4` Run the reclaim/retire pass once per pack, before the existing per-flow upsert loop, so a reclaimed row is already correct by the time that loop's own `GetByPackFlow` lookup runs — no second code path needed there.
- `V-5` Implement as an optional capability (`builtinStaleMirrorReclaimer`, type-asserted), not a new required method on `FlowDefinitionStore` — a backend that doesn't support it (the in-memory test fake) keeps the pre-fix behavior unchanged, so no existing test needed to change.

### Constraints

- Scoped to the mirror-*sync* path (`FlowMirrorSyncService.SyncBuiltins`, which runs at runner startup — the exact trigger the owner's repro used). The lazy per-run `FlowDefinitionResolver.ResolveBuiltin` path (used when a flow is actually selected to run) is unchanged in this fix; it still falls back to a plain re-insert if the mirror is missing at that point, same as before. In practice `SyncBuiltins` already runs on every startup, so a corrupted row is reclaimed/retired well before `ResolveBuiltin` would ever need to fall back.
- Does not retroactively fix an *already*-duplicated pair already sitting in a live database beyond the code-level cleanup: the orphan is retired (stops showing as a duplicate built-in) on the next sync, but the already-inserted correct row's `model_override` (already defaulted to `gpt-5.4` from the earlier bad insert) is not rewritten — there is no longer a source of truth for what it "should" be once the duplicate has already formed. The owner should confirm the recreated row's model override still matches their intent (Settings → Workflows) after upgrading.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go` `Upsert` (line ~297, `on_conflict=pack_id,pack_flow_id`), `ReclaimOrRetireStaleBuiltinMirrors` (new).
- `apps/local-runner/internal/runner/flow_definition_resolver.go` `FlowMirrorSyncService.SyncBuiltins` (line ~293).
- `supabase/migrations/20260525140000_backfill_ai_model_and_reasoning_defaults.sql` (line 10, `model_override` column default `'gpt-5.4'`).
- Owner-provided evidence: two `workflows` table CSV exports from the live Supabase project, captured before and after manually renaming `pack_flow_id` on the `review-loop` row and restarting the runner.

## 1. Issue Summary

While manually executing CP-36 Scenario 14 against a real backend, the owner corrupted the built-in Review Loop mirror row's `pack_flow_id` and restarted the runner to trigger the documented self-heal. The self-heal did fire and did produce a correct new mirror row (Scenario 14's own 4 checklist items all passed), but the corrupted row was never cleaned up, so the workflow list and the Workflow Mode picker both showed two "Review Loop / BUILT-IN" entries — confirmed visually in the owner's screenshots of `WorkflowsSettings.tsx`'s Definitions list and the Workflow-picker dropdown. Separately, the newly-created row's `model_override` silently reverted from the owner's original `claude-haiku` to the column's raw default `gpt-5.4`.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 14 ("Missing Built-in Mirror Row Is Recreated On Demand") — this bug was found *while* manually executing that scenario's own checklist; none of its 4 items call out duplicate-row or override-preservation behavior, which is exactly the gap this bug closes.
- impacted tech design: none directly (`SD-19`/`SS-16` describe the resolver's normalized-shape contract, not this recreate edge case).

## 3. Environment and Reproduction

- environment: Desktop app + local-runner against a real Supabase project (not the mock/offline dev sandbox).
- reproduction steps:
  1. In the `workflows` table, rename the built-in `review-loop` row's `pack_flow_id` (e.g. to `review-loop-haha`), leaving everything else (including `pack_hash`) untouched.
  2. Restart the local-runner process.
  3. Inspect the `workflows` table: a new row appears with the correct `pack_flow_id=review-loop`, `is_builtin=true`, matching `pack_hash`/`pack_version` — but the corrupted row is still present, and the new row's `model_override` is the column default rather than the corrupted row's prior value.
  4. In Settings → Workflows (or the Workflow Mode picker), observe two "Review Loop" entries both badged "Built-in".
- frequency: deterministic — reproduced identically across the owner's two captured CSV snapshots (before/after).

## 4. Expected vs Actual

- expected (per Scenario 14's intent, generalized slightly beyond its literal 4 checklist items): a corrupted/renamed builtin mirror row is repaired or superseded cleanly — never left behind as a second, indistinguishable "Built-in" entry, and never causes an unrelated per-installation setting (`model_override`) to silently reset.
- actual: the corrupted row survives forever (nothing ever queries for or cleans up an orphaned `is_builtin=true` row), producing a permanent UI duplicate; and because `model_override`/`provider_override`/`reasoning_effort_override`/`yolo_mode` are outside `FlowDefinitionRecord`/the pack schema, the fresh `INSERT` this bug's root cause produces falls through to the raw Postgres column default instead of anything meaningful.

## 5. Impact

- users affected: any project using real Supabase-backed built-in flows where a mirror row's `pack_flow_id` is ever corrupted (manual DB edit, migration bug, partial write) or the row is deleted outright.
- workflows affected: Settings' Definitions list, the Workflow Mode picker, and — for the override-loss half — every future run of the affected built-in flow (wrong model silently in effect).
- severity: medium — not a crash or execution-correctness bug (the correct mirror does exist and does run correctly), but a confusing, permanent, and easily-missed data-integrity defect once it happens.

## 6. Root Cause

- hypothesis: the recreate path treats "no row currently matches this pack identity" as always meaning "never synced before," with no way to distinguish that from "the row exists but its identity key was corrupted."
- confirmed cause: `SupabaseWorkflowFlowStore.Upsert`'s builtin branch (`supabase_workflow_flow_store.go:297`) posts to `/workflows?on_conflict=pack_id,pack_flow_id`. PostgREST's `on_conflict` upsert only updates a row when an existing row's *current* column values collide with the incoming ones on that unique index. Once the target row's `pack_flow_id` value has been changed away from `review-loop`, no row conflicts with `(flowpilot-core-flow-pack, review-loop)` — Postgres performs a plain `INSERT`, producing a distinct row with a new `id`. The old row, no longer matching the conflict key, is left completely untouched (both to fetch and to modify) by anything in the sync/resolve paths. Compounding this, `Upsert`'s payload (`supabase_workflow_flow_store.go:298-313`) never includes `model_override`/`provider_override`/`reasoning_effort_override`/`yolo_mode` at all — those columns are entirely outside `FlowDefinitionRecord` and the pack schema — so a genuine `UPDATE` leaves them alone (fine), but a fresh `INSERT` has nothing to seed them with beyond the column's own `DEFAULT` (`workflows.model_override DEFAULT 'gpt-5.4'`, set by `supabase/migrations/20260525140000_backfill_ai_model_and_reasoning_defaults.sql:10`).
- evidence: the owner's two live CSV exports show, byte-for-byte, the corrupted row (`pack_flow_id=review-loop-haha`, `model_override=claude-haiku`, unchanged `pack_hash`) still present after restart, alongside a brand-new row (`pack_flow_id=review-loop`, `model_override=gpt-5.4`, the *same* `pack_hash` as the corrupted row — proving the content was never actually different, only the identity key).

## 7. Fix Strategy

- `F-1` Add `SupabaseWorkflowFlowStore.ReclaimOrRetireStaleBuiltinMirrors(ctx, packID, currentFlows []flowHashID)`: lists every `is_builtin=true` row for `packID`, and for each row whose `pack_flow_id` doesn't match any of `currentFlows`, either (a) patches it in place (`pack_flow_id` only, by the row's own `id`) when its `pack_hash` matches a current flow's hash and that flow's slot isn't already occupied by another row, or (b) retires it (`is_builtin=false`, `editable=true`, name suffixed `" (stale mirror — pack_flow_id no longer matches any current pack flow; safe to review/delete)"`) otherwise — including the case where the duplicate has already formed.
- `F-2` Expose this as an optional `builtinStaleMirrorReclaimer` interface, type-asserted from `FlowMirrorSyncService.SyncBuiltins` (`flow_definition_resolver.go`) and invoked once per pack, before the existing per-flow upsert loop — so a reclaimed row already satisfies that loop's own `GetByPackFlow` freshness check with no separate branch needed there.
- `F-3` Leave `FlowDefinitionResolver.ResolveBuiltin`'s lazy per-run fallback path unchanged (see Constraints) — the startup sync this fix targets already runs on every process start, which is the trigger both Scenario 14 and the owner's repro actually use.

## 8. Validation

- `V-1` `go build ./...` — clean.
- `V-2` New tests in `supabase_workflow_flow_store_test.go`: `TestReclaimOrRetireStaleBuiltinMirrorsReclaimsByHashWhenSlotIsFree` (corrupted row alone → repaired in place, `is_builtin` untouched), `TestReclaimOrRetireStaleBuiltinMirrorsRetiresWhenSlotAlreadyOccupied` (reproduces the owner's exact live-DB state — orphan + already-correct row both present → orphan retired, occupant untouched), `TestReclaimOrRetireStaleBuiltinMirrorsRetiresWhenNoHashMatch` (content genuinely changed/removed → retired), `TestReclaimOrRetireStaleBuiltinMirrorsSkipsAlreadyRetiredRows` (idempotency — no repeated patch). All pass.
- `V-3` New tests in `flow_definition_resolver_test.go`: `TestFlowMirrorSyncInvokesStaleMirrorReclaimerBeforeUpserting` (spy confirms `SyncBuiltins` calls the reclaimer exactly once per pack with the full current flow id+hash set), `TestFlowMirrorSyncWorksWithoutStaleMirrorReclaimerSupport` (a store without the optional capability is unaffected). Both pass.
- `V-4` `go test ./internal/runner/...` (full package): 15 pre-existing, environment-specific failures (missing real Codex CLI, Windows-specific home-dir/path assertions, provider-home skill-precedence tests needing real files on this machine) — same count as the established baseline, none newly introduced.
- `V-5` `go vet ./internal/runner/` — clean.
- `V-6` Not performed: a live round-trip against the owner's actual Supabase project. All verification above is against the mocked `httpRequestFn` HTTP layer (this repo's existing test convention for this store); the owner should restart their local-runner (a fresh `bin/flowpilot.exe` was built this session) and confirm the corrupted `review-loop-haha` row flips to `is_builtin=false` with the stale-mirror name suffix on next startup.

## 9. Regression Guard

- tests: the 6 new tests listed under Validation `V-2`/`V-3` are the permanent regression guard for both the duplicate-row and the reclaim-vs-occupied-slot cases.
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread; proceeded via direct code inspection per the `add-new-bug` skill's fallback instruction.

## 10. Follow-Up Document Updates

- upstream docs that must change: none. Scenario 14's own 4 checklist items in CP-36 remain accurate as written (they describe the recreate happening, which is still true) — this bug documents and closes a gap *alongside* that scenario rather than changing its acceptance criteria.
- notes left unchanged on purpose: `FlowDefinitionResolver.ResolveBuiltin`'s lazy-fallback comment (`flow_definition_resolver.go:160-169`, "a recreate failure falls through to the plain embedded-pack record rather than failing resolution") is still accurate — this fix does not change that path's behavior, only the startup-sync path's.
