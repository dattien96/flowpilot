# CA-1046 — Scaffold progress feed scoped to the selected binding (BUG-549)

## Summary

Desktop Engine Settings renders one "AI Scaffold" transcript card per project,
keyed by `projectId` alone — so a scaffold run dispatched on binding `mac`
still rendered when the operator switched the Binding dropdown to `Primary`,
making it look like the run targeted the wrong path. Dispatch itself was always
correct (`workingDirectory` is sent); only the feed lacked a binding scope.

The same root cause also broke restart-time replay for non-primary bindings:
`loadScaffoldProgressTail` resolved `project.Path` (binding[0]) instead of the
directory the run actually targeted.

## Changes

- `internal/runner/scaffold_progress.go`
  - `handleScaffoldProgress`: optional `workingDirectory` query param; resolved
    through `resolveEngineWorkingDirectory` when possible, else kept raw (an
    unresolvable filter simply matches nothing — fail-closed).
  - `scaffoldProgressSnapshotScoped` (new): hub events are served only when
    `sameWorkspacePath(hub.workspace, filter)`; a mismatched filter skips the
    hub and replays the filtered directory's persisted tail.
  - `scaffoldProgressSnapshot(projectID, after)` signature unchanged — it
    delegates with an empty filter, preserving the legacy project-level feed
    for TUI and Projects Settings (oracle-rule: existing test call sites
    untouched).
  - `loadScaffoldProgressTail(projectID, workspaceFilter)`: explicit filter
    wins over `project.Path`.
  - `sameWorkspacePath` (new): Abs+Clean comparison with an EvalSymlinks
    fallback (macOS /var → /private/var).
- `apps/desktop-flowpilot/src/components/settings/projectEngine.ts`
  - `fetchScaffoldProgress` gains an optional `workingDirectory` arg, appended
    as a query param when present.
- `apps/desktop-flowpilot/src/components/settings/ScaffoldActivity.tsx`
  - Optional `workingDirectory` prop; forwarded to every poll; the effect now
    depends on it and clears the rendered transcript immediately on change.
- `apps/desktop-flowpilot/src/components/settings/EngineSettings.tsx`
  - Passes `selectedBindingPath` so the card follows the inspected binding.
- `ProjectsSettings.tsx` intentionally untouched — that surface has no binding
  selector, so the project-level feed remains the right scope there.

## Tests (RED→GREEN)

- `TestBug549_ProgressFilterHidesOtherWorkspaceFeed` — red (dir-B poll leaked
  dir-A's 6 events); post-fix: filtered poll empty/inactive, matching filter
  returns the run, no filter keeps the legacy feed.
- `TestBug549_ProgressFilterReplaysFilteredWorkspaceTail` — red (replay pinned
  to `project.Path`); post-fix: `?workingDirectory=B` replays B's persisted
  NDJSON while the unfiltered poll still resolves `project.Path`.
- `scaffoldActivity.binding-scope.test.ts` — `fetchScaffoldProgress` appends
  `workingDirectory` when given, omits it otherwise.

## Verification

- `go test -count=1 ./internal/runner/ -run 'Bug549|ScaffoldProgress|ScaffoldDispatch|ScaffoldStatus|CreateProject_'` — all PASS.
- Full `internal/runner` suite: only pre-existing env failures/flakes
  (`TestFirebaseToolsMcpAdapterFetchEndToEnd`, `TestCatalogStoreForFallsBackToFake`,
  `TestCleanupSessionsTearsDownProviderPools`, `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil`
  fail identically on clean HEAD; `TestBug516` flakes both ways) — none in the
  scaffold path.
- Desktop `tsc --noEmit` clean; compiled phase1 tests for the scaffold feed
  pass (7/7).
- Provider parity: transport/feed change only — no provider-specific code.
- `gitnexus` MCP unavailable in this session; impact verified manually —
  `scaffoldProgressSnapshot` callers: `handleScaffoldProgress` + existing
  tests (kept source-compatible); `loadScaffoldProgressTail` is single-caller.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: BUG-549
change_type: bugfix
summary: scope the scaffold progress feed to the selected binding — workingDirectory filter on /scaffold/progress, hub matched by workspace, filtered persisted-tail replay, EngineSettings passes the binding path
# --->8---
