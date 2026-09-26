# CA-1014 — BUG-513: Claude result usage accumulates into `Total`

## Context

Deep review B-3: `mapClaudeResult` emitted only `TokenUsage.Last`;
cap/compaction read `Total`. Claude `print` respawns per turn so result
usage is query-scoped — a verbatim `Total` would under-report later
queries.

## Changes

- `claude_event_mapper.go`: result usage emits `Last` + `Total`, marked
  `QueryScoped`.
- `provider_event.go`: `TokenUsage.QueryScoped` flag.
- `emitLocked` (`interactive_service.go`): accumulates query-scoped
  totals per provider session (`rs.legUsageTotals`), seeds from durable
  prior events post-restart, restamps `Total` with the accumulated
  value; new `providerSessionID` starts a fresh bucket; cumulative
  (non-query-scoped) events pass through unchanged.
- `bug513_claude_usage_total_test.go`: mapper emits query-scoped Total;
  same-session accumulation; leg reset; restart seeding; non-scoped
  passthrough.

## Verification

`go test -count=1 -run TestBug513 ./internal/runner/` — green.
