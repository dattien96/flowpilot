# CA-983 — BUG-474 follow-up: Desktop-path clones heal `definition_json` from `cloned_from` provenance

## Change ID
CA-983

## Date
2026-10-18 (live-test follow-up to CA-981-era BUG-474 work; found during Leg A of the full live-test phase)

## Scope
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`
- `apps/local-runner/internal/runner/bug474_cloned_flow_execution_fields_test.go`
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`

## Summary
The original BUG-474 fix guaranteed `definition_json` on rows written through
the runner's `Upsert` and restored schema-less execution fields on read. The
live leg then exposed a second write path that bypasses the runner entirely:
`supabaseAdminRepository.cloneWorkflow` INSERTs the `workflows` row through
PostgREST directly, so a Desktop-created clone lands with `cloned_from` set
but `definition_json` NULL and silently drops `run`, `posture`,
`contextProfile`, `config`, `model`, flow `tools`, and `contextProfiles` —
the exact regression BUG-474 was meant to kill, just one hop later.

This change closes the loop from both sides:

1. **Read-time heal (runner).** `healCloneDefinitionSnapshot` runs on every
   `fetchOne`/`List` for a row that is non-builtin, has `cloned_from`, and
   has no `definition_json`. It resolves provenance one hop: the source
   row's own `definition_json` (clone-of-user-flow) or its embedded pack
   flow via `embeddedFlowDefinition` (clone-of-builtin-mirror). The
   recovered snapshot is applied with the same fill-only precedence as the
   `definition_json` branch — real columns still win — then PATCHed onto
   the clone row once (`return=minimal`), so later reads take the cheap
   branch. Every failure is soft: missing/unreadable source falls back to
   the deterministic legacy derive-run reconstruction; a failed backfill
   only costs one more source lookup next read.
2. **Write-time passthrough (client-core).** `cloneWorkflow` now copies
   `definition_json` verbatim from the source row when present. A
   clone-of-user-flow keeps its parent's snapshot without waiting for a
   runner read; a clone-of-builtin still writes no key (mirror rows carry
   none) and is covered by the heal. The key is conditional because
   PostgREST rejects unknown columns outright — unconditionally sending
   `definition_json` would break cloning on projects that have not run the
   migration yet.
3. **Unmigrated-remote degradation (runner + client).** Live verification
   against the test bed's real Supabase project showed
   `workflows.definition_json` does not exist there yet (42703). Three
   degrades keep pre-migration deployments working with the pre-fix field
   set instead of hard-failing: the heal's source select retries without
   the column (builtin-source heal needs only pack identity), `Upsert`
   strips the snapshot key and retries once with a loud log line, and the
   backfill PATCH soft-skips. A per-store `atomic.Bool` latches on the
   first 42703 so steady-state reads don't pay extra round-trips.

## Why this design
- Chained clones are handled transitively: healing a child backfills its
  snapshot, so a grandchild heals from the child's snapshot on read.
- The heal is bounded to one extra GET per legacy/Desktop-clone row read,
  amortized to zero by the one-time backfill.
- No migration needed — `definition_json` already exists from the
  original BUG-474 fix.

## Evidence
- RED: `TestBUG474_DesktopCloneRowHealsFromBuiltinSource`,
  `TestBUG474_DesktopCloneRowHealsFromUserSourceSnapshot` — posture/
  contextProfile came back empty before the heal.
- GREEN: all 8 `TestBUG474_*` pass.
- `TestBUG474_DesktopCloneWithMissingSourceKeepsLegacyPath` pins the
  soft-fail contract (deleted source → legacy derive-run, no error).
- `TestBUG474_LegacyCloneWithoutSnapshotDerivesRun` still passes — rows
  without `cloned_from` are untouched.
- Desktop `tsc --noEmit`: clean.
- Full `go test ./internal/...` — pending; env-baseline list unchanged.

## Risk
- Low. Heal is additive (fill-only) on a code path that previously returned
  degraded definitions anyway. The PATCH is `id`-scoped, `return=minimal`,
  and idempotent — concurrent reads of the same clone may PATCH twice
  with identical content, which is harmless.

## Rollback
Revert the heal call sites (`fetchOne`, `List`), `healCloneDefinitionSnapshot`,
the `definition_json` line in `cloneWorkflow`, and the three new tests.
