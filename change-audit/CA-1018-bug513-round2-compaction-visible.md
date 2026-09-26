# CA-1018 — BUG-513 round 2: Claude compaction observable via Last

## Context

Round-2 review: accumulating query-scoped usage into `Total` fixed cap
accounting but kept `isProviderCompaction` blind — the detector needs a
DROP in the tracked figure and the accumulated total only grows.

## Changes

- `context_pressure.go` `evalContextPressureLocked`: when the leg is
  query-scoped (membership in `rs.legUsageTotals`, populated by the emit
  seam before eval runs), the tracked figure is `snap.Last.TotalTokens` —
  the query's own context read — so a provider-side compaction shows as
  the sharp same-leg drop it is. Cumulative legs still track `Total`.

## Tests

3 new cases in `bug513_claude_usage_total_test.go` — Last-drop on a
query-scoped leg emits `provider_compacted` + marks the leg
context_degraded; growth is not flagged; cumulative legs keep the Total
drop semantics.

## Parity

Only Claude marks `UsageScopeQuery`; Codex/Devin/Grok cumulative frames
are structurally untouched (verified: the override only applies inside
the query-scoped branch).
