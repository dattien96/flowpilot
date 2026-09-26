# BUG-513 — Claude result usage never populates `Total`; cap/compaction blind to Claude spend

## Status
FIXED — 2026-09-26 (CA-1014).

## Found during
Deep review B-3 (`CP-Deep-Review-12CP-2026-09-26.md`), verified against
source: `mapClaudeResult` populated only `TokenUsage.Last`, while
context_usage/compaction reads `Total` as the latest cumulative
per-session figure. Existing parity tests injected `Total` directly and
never exercised the real mapper.

## Observed

Claude's `print` adapter respawns per turn, so each result frame's usage
is *query-scoped*, not session-cumulative — unlike devin/codex/grok whose
sessions emit true cumulative totals. Writing the per-query figure into
`Total` verbatim would under-report (later queries overwrite), and
summing assistant-event usage would double-count against the result's
query aggregate. Net effect: Claude spend was invisible to both the
usage cap and compaction triggers.

## Fix (CA-1014)

- `mapClaudeResult` marks result usage `QueryScoped: true` and emits
  both `Last` and `Total` (per-query).
- `emitLocked` accumulates query-scoped `Total` values per provider
  session (`rs.legUsageTotals`), seeds the bucket from durable prior
  events so it survives restart, and restamps the event's `Total` with
  the accumulated value.
- A new provider session/leg (`providerSessionID` change) starts a fresh
  bucket — matching the per-session semantics the consumers assume.
- Non-query-scoped events (codex/grok/devin cumulative frames) are
  passed through unchanged — no provider-parity drift.

## Tests

`bug513_claude_usage_total_test.go` — real mapper emits query-scoped
Total; two same-session result frames accumulate; a new leg resets the
bucket; durable prior totals seed after restart; non-query-scoped
events untouched. Green.

## Round 2 (2026-09-26, CA-1018) — compaction is now observable on Claude

The first fix accumulated per-query usage into a session `Total` for cap
accounting — but `isProviderCompaction` needs a DROP in the tracked
figure, and the accumulated total can only grow, so the detector stayed
blind on Claude.

Round 2: `evalContextPressureLocked` detects the compaction signature on
query-scoped legs via `snap.Last` — the query's own context read — whose
sharp same-leg drop IS what a provider-side compaction looks like. The
leg's membership in `legUsageTotals` (set by the accumulation seam before
evaluation) marks it query-scoped; cumulative legs (Codex/Devin/Grok)
keep tracking `Total` unchanged.

## Tests (round 2)

- `TestBug513_QueryScopedLegDetectsCompactionViaLast` — accumulated
  Total grew 1000→1400 while Last dropped 1000→400 → provider_compacted
  emitted + leg marked context_degraded.
- `TestBug513_QueryScopedLegNoFalsePositiveOnGrowth` — growing per-query
  context does not flag compaction.
- `TestBug513_CumulativeLegStillTracksTotalDrop` — cumulative legs still
  detect the Total drop.

## Round 3 (2026-09-26, CA-1024) — near-window gate kills the false positive

Grok review: a per-query figure ALSO drops on an ordinary shorter turn —
the bare >30% `Last`-drop detector flagged `provider_compacted` for a
normal 500→300 sequence on a half-full window. The provider only compacts
a near-full context window, so the round-3 gate: a query-scoped drop
counts as compaction only when the PREVIOUS read was already at the
≥80% pressure tier (`snap.ModelContextWindow` → `legContextWindows`).
Unknown window → stays blind rather than guessing (same stance as the
pressure ladder). Cumulative legs unchanged.

The round-2 fixture (Last 1000→400, no window) was itself the
false-positive shape — updated to a 90%-full prev so the positive case
still locks.

## Tests (round 3)

- `TestBug513_QueryScopedLegDetectsCompactionViaLast` — updated fixture:
  window 1000, Last 900→400 (prev at pressure tier) → still flags.
- `TestBug513_QueryScopedLegShortTurnIsNotCompaction` — window 1000,
  Last 500→300 (>30% drop, prev only 50%) → NOT flagged, no degrade mark.
- `TestBug513_QueryScopedLegUnknownWindowStaysBlind` — no window data →
  the same 1000→400 shape does not flag.

## Round 4 (2026-09-26) — query-scoped legs stay blind to compaction

Grok review round 4: the round-3 gate is still unsound. `snap.Last` for
Claude is the single query's token aggregate, NOT session-window
occupancy. A long query (prev ≥80% of window) followed by a short query
still satisfies "prev near-full + >30% drop" without the provider
compacting anything — per-query size cannot prove context shrinkage, and
there is no reliable live signal to validate the mapper's accounting
against (no connected Claude account on this machine).

Fix: compaction detection is limited to legs whose `Total` is
session-cumulative (Devin/Grok/Codex emit true cumulative figures). A
leg marked query-scoped (present in `legUsageTotals`, set by the
accumulation seam) never emits `provider_compacted` and never gets
marked `context_degraded`. Query-scoped `Total` still accumulates for
cap accounting — that half of the original fix is untouched. Claude's
`Last`/`ModelContextWindow` still feed the pressure ladder (aware/ask
events) — that ladder asks the operator rather than claiming compaction,
which remains honest.

## Tests (round 4)

- `TestBug513_QueryScopedLegStaysBlindToPerQueryDrop` — window 1000,
  Last 900→400 (Grok's exact false-positive shape) → NOT flagged, leg
  not degraded, accumulated Total still 1300.
- `TestBug513_QueryScopedLegNoFalsePositiveOnGrowth`,
  `TestBug513_QueryScopedLegShortTurnIsNotCompaction`,
  `TestBug513_QueryScopedLegUnknownWindowStaysBlind` — unchanged, green.
- `TestBug513_CumulativeLegStillTracksTotalDrop` — cumulative detection
  preserved.
- `TestTask443_*` compaction fixtures (cumulative) — all green.

Residual: Claude-side compaction is now genuinely undetected — the
correct behaviour until a real session-fullness signal exists. Unit-only;
no live Claude account exists to verify against.
