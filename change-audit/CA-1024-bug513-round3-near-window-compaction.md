# CA-1024 — BUG-513 round 3: query-scoped compaction gated on near-full window

## Context

Grok review round 3: the round-2 fix tracked `snap.Last` for compaction
on query-scoped (Claude) legs — but a per-query figure also drops on an
ordinary shorter turn. A bare >30% drop flagged `provider_compacted` for
a normal 500→300 sequence on a half-full window, and marked the leg
`context_degraded` spuriously. (Dormant today — FLOWPILOT_CONTEXT_PRESSURE
is default-off — but a real mis-fire when enabled.)

## Changes

- `context_pressure.go` `evalContextPressureLocked`: on query-scoped
  legs the Last-drop counts as compaction only when the previous read
  was already at the ≥80% pressure tier — a provider compacts a
  near-full window, not a half-full one. Window resolved from
  `snap.ModelContextWindow` → `rs.legContextWindows[legID]`; unknown →
  stays blind (same "never guess" stance as the pressure ladder).
  Cumulative legs unchanged.

## Tests

Updated fixture for `TestBug513_QueryScopedLegDetectsCompactionViaLast`
(my round-2 file — the old fixture was itself the false-positive shape);
new `TestBug513_QueryScopedLegShortTurnIsNotCompaction` and
`TestBug513_QueryScopedLegUnknownWindowStaysBlind`.
