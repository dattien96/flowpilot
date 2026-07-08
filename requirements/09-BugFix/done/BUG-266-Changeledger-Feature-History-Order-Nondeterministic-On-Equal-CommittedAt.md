# BUG-266: `changeledger.GetFeatureHistory` Order Non-Deterministic When `CommittedAt` Values Are Equal

## Metadata

- Document ID: `BUG-266`
- Title: `changeledger.GetFeatureHistory Order Non-Deterministic When CommittedAt Values Are Equal`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md) (D-3 — "newest = current truth")
- Child Documents: `None`
- Related Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md), [Task-192: Migrate Built-in Context Sources](../../08-Task/todo/Task-192-Migrate-Builtin-Context-Sources.md) (discovered while writing its behavior-preserving golden test)
- Replaces: `None`
- Tags: `changeledger, determinism, feature-history, context-regression-engine, map-iteration`

## AI Quick View

### Summary

- `Ledger.entries` is a `map[string]Entry` keyed by commit hash; `AllEntries()` ranges over it, and Go intentionally randomizes map-iteration order on every `range` call.
- `GetFeatureHistory`'s `sort.Slice` comparator only ordered by `CommittedAt`; two entries with equal (including both-empty) `CommittedAt` were "equal" to the sort, so their final relative order silently depended on that random map-iteration order.
- Net effect: for a feature whose commits share a `CommittedAt` granularity (or never had it populated), which commit is presented as "current truth" (the last entry, per SD-17 D-3) could flip between two different commits across separate calls against the exact identical ledger data.
- Discovered incidentally: a byte-exact golden test written for CP-44/Task-192 (behavior-preserving refactor) was the first test in the codebase strict enough to expose it — no existing test asserted exact ordering.

### Current Ask

- Make `GetFeatureHistory`'s ordering fully deterministic regardless of `Ledger.entries`'s map-iteration order.

### Key Decisions

- `V-1` Add `CommitHash` as a secondary, deterministic sort key when `CommittedAt` is equal. `sort.SliceStable` alone is insufficient — it only preserves whatever order `AllEntries` happened to hand it, which is itself random; a content-derived tiebreaker is required to remove the dependency on map iteration entirely.

### Constraints

- Fix must not require callers to populate `CommittedAt`; entries with no timestamp are a valid, already-supported state.
- Must not change ordering for any existing test/fixture that already sets distinct `CommittedAt` values (those are unaffected — the tiebreaker only applies when `CommittedAt` is equal).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/changeledger/ledger.go:34-38` (`Ledger.entries map[string]Entry`, `AllEntries`).
- `apps/local-runner/internal/changeledger/query.go:10-31` (`GetFeatureHistory`, the fixed function).
- `apps/local-runner/internal/runner/flow_context_package_test.go` (`fcpFixture`) — a shared test fixture that had to add explicit distinct `CommittedAt` values once this bug started intermittently failing a stricter downstream test; kept even after this fix as good fixture hygiene.

## 1. Issue Summary

`changeledger.GetFeatureHistory(featureKey)` is documented and relied upon (SD-17 D-3) to return a feature's commit history "oldest → newest, newest = current truth" deterministically. Because the underlying entry store is a Go map and the sort had no tiebreaker for equal `CommittedAt`, the returned order — and therefore which entry is treated as "current truth" — was not actually deterministic across repeated calls with identical underlying data.

## 2. Parent Links

- impacted coding plan: `CP-44` (surfaced this while adding a byte-exact regression test)
- impacted tech design: `SD-17` (D-3: ordered commit history, newest = current truth)
- impacted system spec: `SS-14` (context/regression safety depends on stable "current truth")

## 3. Environment and Reproduction

- environment: any bound project using `.flowpilot/ledger/feature_history.ndjson` where a feature has ≥2 commit entries with equal `CommittedAt` (including the common case of `CommittedAt` never being set).
- reproduction steps:
  1. `Upsert` two or more entries for the same `feature_key` without setting `CommittedAt` (or with equal values).
  2. Call `GetFeatureHistory(featureKey)` repeatedly (each call constructs a fresh iteration over the underlying map).
  3. Observe: relative order of equal-`CommittedAt` entries is not guaranteed stable across calls.
- frequency: intermittent — depends on Go's per-`range`-call randomized map iteration seed; reproduced deterministically in a loop of 50 repeated calls in the regression test (`TestGetFeatureHistoryDeterministicForEqualCommittedAt`), and was observed to fail Task-192's golden test in roughly half of local runs before the fix.

## 4. Expected vs Actual

- expected: for identical ledger data, `GetFeatureHistory` returns entries in the same order on every call; the "current truth" (newest) entry is stable.
- actual: entries with equal `CommittedAt` could be returned in either relative order depending on the Go runtime's randomized map-iteration order for that particular call.

## 5. Impact

- users affected: any Flow Mode context-harness consumer relying on "newest = current truth" (Plan step's `feature.history` context source, chat-mode feature-history injection).
- workflows affected: CP-41 RAG Harness context assembly, CP-35 chat-mode history injection — both read `GetFeatureHistory`/`HistorySlot` output as ground truth for what the AI should build on.
- severity: medium — silent and data-dependent (only manifests when `CommittedAt` values collide), but directly undermines the "newest = current truth" invariant SD-17 D-3 and CP-41's context-harness design depend on.

## 6. Root Cause

- hypothesis: none needed — root cause fully identified via code inspection.
- confirmed cause: `Ledger.entries` is `map[string]Entry` (`ledger.go:37`); `AllEntries()` ranges over it (`ledger.go:110-114`); Go randomizes map iteration order per `range` call by design. `GetFeatureHistory`'s `sort.Slice(matched, func(i,j) bool { return matched[i].CommittedAt < matched[j].CommittedAt })` (pre-fix) has no tiebreaker, so entries with equal `CommittedAt` are "equal" under the sort predicate and an unstable sort (`sort.Slice`) does not guarantee their relative order is preserved from input order — which was itself already random.
- evidence: `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor/verified_feature_history_only` (CP-44 Task-192 golden test) intermittently failed with the two fixture commits swapped, on a fixture where neither entry set `CommittedAt`.

## 7. Fix Strategy

- `F-1` Change `GetFeatureHistory`'s sort to `sort.SliceStable` with a two-key comparator: primary `CommittedAt` ascending, secondary `CommitHash` ascending when `CommittedAt` is equal (`internal/changeledger/query.go`). `CommitHash` is stable, content-derived, and always populated, removing any dependency on map-iteration order.
- `F-2` (Task-192, already-shipped fixture hygiene) The shared test fixture `fcpFixture` (`internal/runner/flow_context_package_test.go`) now sets distinct, explicit `CommittedAt` values for its two seed commits — good practice independent of this fix, since it makes the fixture's intended chronological order explicit rather than incidental.

## 8. Validation

- `V-1` New test `TestGetFeatureHistoryDeterministicForEqualCommittedAt` (`internal/changeledger/query_determinism_test.go`): seeds 5 entries sharing the same (empty) `CommittedAt`, calls `GetFeatureHistory` 50 times, and asserts identical order every time (ascending `CommitHash`).
- `V-2` Full `internal/changeledger` suite (27 tests) passes unchanged — no existing ordering assertion (all of which use distinct `CommittedAt` values) is affected by the tiebreaker.
- `V-3` `internal/runner`'s CP-44 golden test (`TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor`) now passes stably across 5+ repeated `-count=1` runs, where it previously failed intermittently.

## 9. Regression Guard

- tests: `TestGetFeatureHistoryDeterministicForEqualCommittedAt` (loop of 50 calls) guards against any future reintroduction of a map-iteration-dependent sort in this function.
- alerts: none.
- audit checks: none — this is a pure library-level determinism fix with no user-facing audit trail.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — `SD-17 D-3`'s "newest = current truth" invariant is now actually upheld as designed; no design change, only a correctness fix.
- notes left unchanged on purpose: `fcpFixture`'s explicit `CommittedAt` values (Task-192) are kept even after this fix — they make the fixture's intended order self-documenting rather than relying solely on the tiebreaker.
