# CA-436: CP-54 P-6 Symbol Overlap Scorer + Rank Telemetry

**Date:** 2026-08-11  
**Scope:** CP-54 P-6 (symbol tier + auditable rank log)

## Summary

Shipped CP-54 P-6 after BUG-323 / Task-259 unblocked symbol input:

- `HistoryRelevance.SymbolOverlap` — deterministic basename heuristic (mirrors `GitNexusImpactTargets`, duplicated in `featurecatalog` to avoid import cycle).
- Rank order: `PathOverlap` → `SymbolOverlap` → `CommitUnix` → `CommitHash`.
- `buildRetrievalLocus` finalizes `locus.Symbols` via `changecontract.GitNexusImpactTargets` (declared paths + `symbols:`).
- `LogHistoryRankingSelection` — `[context-rank]` lines when ranked history renders (SS-14 AC-7).

**Not in v1:** GitNexus `Dependents` at rank-time (forward blast-radius stays on `source.dependence` / `r-scope`).

## Files

- `apps/local-runner/internal/featurecatalog/relevance.go` — scorer + telemetry
- `apps/local-runner/internal/featurecatalog/slots.go` — call log on ranked path
- `apps/local-runner/internal/featurecatalog/locus.go` — doc update
- `apps/local-runner/internal/runner/retrieval_locus.go` — populate symbols
- `apps/local-runner/internal/changecontract/impact_targets.go` — `DerivedSymbolFromPath` export
- Tests (additive): `relevance_symbol_test.go`, `retrieval_locus_symbol_test.go`; updated obsolete BUG-323 guard in `relevance_test.go`

## Verification

```bash
cd apps/local-runner
go test ./internal/featurecatalog/... -count=1 -run 'Symbol|RankHistoryEntries'
go test ./internal/runner/ -count=1 -run 'TestBuildRetrievalLocusPopulates|TestBuildRetrievalLocusMerges|TestBuildRetrievalLocusDerives'
```

Manual: CP-54-Test-Steps §6.M1–M3.
