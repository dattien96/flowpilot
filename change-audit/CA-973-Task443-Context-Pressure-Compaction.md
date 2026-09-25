# CA-973 — Task-443: context-pressure ladder + provider compaction detection

## Summary

Window-context pressure is a **leg-lifecycle** problem (a provider session
fills up), distinct from account quota (CP-87 routing). Flag-gated by
`FLOWPILOT_CONTEXT_PRESSURE` (default off — byte-identical when disabled):
~80% emits an awareness event, ~90% asks via durable card, a same-leg >30%
token drop reveals provider self-compaction and pins the leg degraded.

## What changed

- `internal/runner/provider_event.go` — `EventContextPressure`,
  `EventProviderCompacted`, `ContextPressurePayload` (tier/ratio/used/window/
  prev/legId).
- `internal/runner/context_pressure.go` — new file:
  - `evalContextPressureLocked` — evaluated inside `emitLocked` on
    `token_usage_updated`; pure in-memory (ratios, per-leg dedupe state on
    the run). Unknown window → silent. Tiers fire once per leg.
  - Tier 80% → `context_pressure{aware}`; tier 90% → durable question
    (`rotate_leg`/`continue`/`stop`) persisted before emit, rollback on
    persist failure. `rotate_leg` option present only on long-lived root
    sessions — per-step child runs get a fresh window next step anyway.
  - Same-leg `Total.TotalTokens` drop >30% → `provider_compacted` +
    `context_degraded` mark on the leg; cross-leg drops ignored.
  - `consumePendingContextReset` — committed rotate intent executes at the
    **next turn-admission boundary only** (never mid-turn); `switchChatLeg
    (allowSameProvider)` mints a fresh leg on the SAME provider+account+
    model; `contextResetHeadroomOK` seam (CP-87 quota preflight — nil
    degrades to allowed).
- `internal/runner/chat_switch.go` — `switchChatProvider` refactored onto a
  shared `switchChatLeg(allowSameProvider)`; normal switches still reject
  same-provider.
- `internal/runner/interactive_service.go` — emitLocked hook, run fields
  (`contextResetPending`, per-leg pressure dedupe), `AnswerQuestion` routing
  for `contextPressureQuestionKind`, admission-boundary consumption in
  `startTurn`, `contextResetHeadroomOK` seam.
- New `task443_context_pressure_test.go` — 12 tests: flag-off byte-identity,
  aware-only at 80%, ask card at 90%, once-per-leg dedupe, unknown-window
  silence, same-leg drop → compacted + degraded, rotate keeps same binding,
  cross-leg drop ignored, child-run rotation suppressed, events persisted to
  the durable store, provider parity.

## Tests

- `go test ./internal/runner/ -run TestTask443` — 12/12 green.

## Honest gaps

- Compaction detection is a heuristic on reported usage — a provider that
  compacts without emitting usage events is missed (documented in Task-443).
- `contextResetHeadroomOK` is nil → allowed until CP-87 supplies the real
  quota feed.
- rotate_leg is suppressed for child runs by design — only the long-lived
  hub session profile benefits (compounding compaction loss).

# ---8<--- flowpilot:change-ledger
feature_key: token-usage
source_doc_id: Task-443
change_type: feature
summary: FLOWPILOT_CONTEXT_PRESSURE-gated ladder — aware 80% event, ask 90% durable card (rotate_leg/continue/stop), same-leg token drop → provider_compacted + degraded mark; rotate_leg = same-binding leg reset at next admission only
# --->8---
