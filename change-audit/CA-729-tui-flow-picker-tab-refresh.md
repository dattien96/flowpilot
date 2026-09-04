# CA-729 — TUI /flow Tab picker refreshes stale cache in background (BUG-351)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-351
change_type: bugfix
summary: Tab /flow picker dispatches a silent background refresh while open (in-flight dedup plus 10s interval bound) so a session-start snapshot taken mid mirror-sync converges instead of sticking; failed refreshes no longer wipe the last good list
# --->8---

## Problem

- Tab `/flow` rendered purely from the cached `flowBuiltins`/`flowWorkflows`,
  and the old maybe-prefetch skipped non-empty caches — a session-start
  snapshot taken while runner mirror-sync was still running (15 rows instead
  of 19) stuck for the whole session. Only Enter `/flow list` fetched fresh.

## Changes

- `apps/local-runner/internal/tui/app/model.go`: new `flowListInflight` +
  `flowListFetchedAt` fields.
- `apps/local-runner/internal/tui/app/app.go`:
  - `cmdMaybePrefetchFlows`: picker open + non-empty cache dispatches silent
    `cmdFetchFlows(true)` unless in-flight or within
    `flowPickerRefreshInterval` (10s); stale cache keeps showing meanwhile.
  - `ConnectedMsg` prefetch, empty-cache refresh, and Enter `/flow list`
    all mark in-flight.
  - `FlowListMsg`: always clears in-flight; replaces the cache only when
    `CatalogErr == ""` (error text output unchanged); stamps fetch time on
    success.
- `apps/local-runner/internal/tui/app/bug351_flow_picker_refresh_test.go`
  (new): stale-cache dispatch-once + merge, error-keeps-cache, interval
  bound.
- `requirements/09-BugFix/todo/BUG-351-*.md`: status done with fix notes.

## Verification

- 3 new BUG-351 tests PASS.
- `go test ./internal/tui/app/ -run 'TestFlow|TestSlash|TestLaunch'`:
  54/54 PASS, zero pre-existing edits (incl.
  `TestFlowListMsg_ShowsBuiltinsWhenCatalogFails` — on-error message text
  unchanged, only the cache is now preserved).
- Live-verify pending: rebuild TUI, restart mid-sync, Tab shows partial then
  converges without Enter.
