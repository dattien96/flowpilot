# Task-096: Commit-History Ledger

## Metadata

- Document ID: `Task-096`
- Title: `Commit-History Ledger`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-097: Feature Catalog And Resolver](./Task-097-Feature-Catalog-And-Resolver.md), [Task-103: Engine Local Store And Drive Sync](./Task-103-Engine-Local-Store-And-Drive-Sync.md)
- Replaces: `None`
- Tags: `changeledger, git, plane-c, local-runner`

## AI Quick View

### Summary

- Build Plane C: turn `git log` + the commit-id convention + `change-audit/` notes into an ordered, per-feature change history.
- Newest entry = current truth. No symbol graph, no AST hashing.
- First and cheapest CP-35 slice; foundation for the resolver (Task-097) and Drive sync (Task-103).

### Current Ask

- Implement `internal/changeledger/` (`parse`, `enrich`, `store`, `query`) per CP-35 §4.1.

### Key Decisions

- `T-1` `feature_key` resolution priority: CA `§13` block → SS-13 parent-chain → dominant path → doc id (last two = low confidence).
- `T-2` NDJSON store, last-wins by commit hash; order ascending by commit time (newest last).

### Constraints

- No dependency on other P-slices. Read-only on git. Non-fatal and retryable; never block the raw save.

### Open Questions

- Confidence threshold for the path-based `feature_key` fallback.

### Source Refs

- `CP-35 §4.1` (P-1); `SD-17 §3.2`, `D-3`; `SS-14 AC-2`.

## 1. Goal

A `GetFeatureHistory(featureKey)` that returns ordered commit entries (newest last) for a feature on any git repo, built without a code graph.

## 2. Parent Links

- coding plan: `CP-35` P-1
- tech design: `SD-17` §3.2, `D-3`
- system spec: `SS-14` AC-2
- specific upstream ids: `P-1`, `D-3`, `AC-2`

## 3. Trigger

CP-35 is staged cheapest-first; the ledger is the foundation other slices build on (resolver, gate context, sync).

## 4. Exact Change

- `T-1` `parse.go` — `git log` parse (format string, `[Type]`/`Task-|BUG-|CP-` regexes, ordering, incremental `.cursor`) per CP-35 §4.1.
- `T-2` `enrich.go` — `feature_key` resolution priority + confidence flag.
- `T-3` `ledger.go` / `store.go` — `Entry` type + NDJSON store at `.flowpilot/ledger/feature_history.ndjson`, last-wins, mutex-guarded.
- `T-4` `query.go` — `GetFeatureHistory`, `ListFeatures`.
- `T-5` unit tests per CP-35 §7 (parse, ordering, cursor, enrich).

## 5. Touched Areas

- files: `apps/local-runner/internal/changeledger/*`
- modules: `changeledger`
- routes: none
- tables: none in this task (mirror added in P-8 / CP-35 §6)

## 6. Acceptance Check

- CP-35 P-1 DoD: ordered entries newest-last on a no-spec repo from git alone; incremental cursor works; CA `§13` enrich applied when present.
- `go build ./...` and `go test ./internal/changeledger/...` pass.

## 7. Out of Scope

- Feature resolution (Task-097), Drive sync (Task-103), GitNexus/structure, any symbol-level work.

## 8. Completion Notes

- result: implemented — `apps/local-runner/internal/changeledger/` (5 files, 22 unit tests, all passing)
- files delivered: `ledger.go` (Entry type, Ledger store, Build), `parse.go` (git log, cursor), `enrich.go` (CA §13 block, FEATURE-KEYS, path-based fallback), `query.go` (GetFeatureHistory, ListFeatures, LatestEntry), `ledger_test.go`
- `go build ./...` and `go test ./internal/changeledger/...` pass
- follow-ups: feeds Task-097 (catalog seeds from FEATURE-KEYS.md + changeledger entries) and Task-103 (syncs feature_history.ndjson to Drive)
- upstream docs updated: none required
