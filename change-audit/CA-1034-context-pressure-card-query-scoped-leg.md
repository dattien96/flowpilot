# CA-1034 — context-pressure ladder no longer fires on query-scoped legs

Date: 2026-09-26 — review finding (round 5): a ≥90% `context_pressure_90`
card could open from a query-scoped leg.

## Change

`apps/local-runner/internal/runner/context_pressure.go`:

- `evalContextPressureLocked` hoists the `queryScoped` check (membership in
  `rs.legUsageTotals`, populated by the BUG-513 accumulation seam) ahead of
  the pressure ladder and returns early for query-scoped legs. On a
  query-scoped provider (Claude `print` respawns per turn), `Last` is the
  single query's token aggregate — not session occupancy — so an aware/ask
  event claiming "this session is at N% of its context window" is a false
  claim, and the `rotate_leg` option is a no-op reset (the provider session
  already starts fresh next query). Same posture as round-4 compaction
  blindness: no session-fullness signal exists, so nothing is asserted.
- Cap accounting is untouched: query-scoped `Total` still accumulates
  session-wide for the usage-cap ledger. Compaction detection remains
  limited to cumulative legs (unchanged from CA-1025).

## Why

The round-4 fix (CA-1025) made query-scoped legs blind to
`provider_compacted` but deliberately kept the aware/ask ladder on the
ground that "it asks the operator rather than claiming compaction". The
ask-tier card, however, is not a neutral question — its prompt asserts
session-level pressure and its `rotate_leg` option performs a real leg
reset (cost + churn) that cannot help a per-query provider. A 95% single
query is a prompt-size fact, not session fullness — the honest surface is
no card.

## Tests

`context_pressure_query_scoped_card_test.go` (new, red-first):

- `TestQueryScopedLeg_PressureLadderStaysBlind` — RED before the fix
  (`context_pressure` event emitted on a query-scoped 95% read). After:
  no pressure event, no card, no tier latch.
- `TestQueryScopedLeg_CapAccountingStillAccumulates` — Total still sums
  950+150=1100 for caps.
- `TestQueryScopedLeg_RealTurnOpensNoCard` — live entry: `startTurn` on a
  Claude leg whose adapter emits a query-scoped 95% usage frame through the
  real bridge → `emitLocked` → eval seam → no `context_pressure`, no
  `user_question_required`.
- `TestCumulativeLeg_PressureLadderUnchanged` — Devin cumulative leg at
  95% still emits the ask-tier card (regression pin).

Regression: `go test -count=1 -run 'TestQueryScopedLeg|TestCumulativeLeg_Pressure|TestTask443|TestBug513|TestTask44[02]|TestTask445'`
— green.

## Provider parity

The change is scoped to the query-scoped marker set only by Claude-style
per-query usage frames; cumulative providers (Codex/Grok/Devin/OpenCode)
keep the full ladder + compaction detection — verified by the unchanged
`TestTask443_*` and `TestTask442_ClaudeCodexGrok_Parity` suites.
