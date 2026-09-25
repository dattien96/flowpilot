# CA-970 — Task-440: ModelContextWindow catalog fallback in emitLocked

## Summary

Providers that never self-report their context window (Claude) emitted
`token_usage_updated` events with `ModelContextWindow = nil`, so every
downstream consumer (TUI status line, desktop usage bar, CP-86 pressure
ratios) had to re-derive the window or silently went blank. The emit seam now
fills the field from the provider catalog — one uniform field for all
consumers.

## What changed

- `internal/runner/interactive_service.go` — `emitLocked` (Task-440 T-1):
  when `ev.Type == token_usage_updated` && `TokenUsage.ModelContextWindow ==
  nil`, fill from `modelContextWindowFor(rs.providerKey, rs.modelName)`.
  Pure in-memory catalog lookup — **no I/O under `s.mu`**. Self-reported
  values are never overridden; unknown models stay nil (degrade-silent).
- `internal/runner/context_window.go` — `modelContextWindowFor(providerKey,
  modelName)`: in-memory catalog scan (`providers[].Models[]
  .ContextWindowTokens`), same source the TUI already uses via
  `contextWindowForModel`. Returns nil when the model is unknown.
- New `internal/runner/task440_window_enrichment_test.go` — 5 tests:
  enriched-when-absent, never-overrides-self-reported, unknown-model-stays-
  nil, pure-in-memory-no-IO-under-lock, Claude/Codex/Grok parity.

## Tests

- `go test ./internal/runner/ -run TestTask440` — 5/5 green.
- Pre-existing failures unchanged: provider-count spec (expects 4, catalog
  now has 6) and one env-dependent catalog fallback test both reproduce on a
  clean HEAD worktree — recorded, not touched (additive-tests-only).

## Honest gaps

- Enrichment keys on `(providerKey, modelName)` — a model name shared across
  catalogs resolves to the first match; provider-scoped models are exact.
- GitNexus `impact` on `emitLocked` = CRITICAL (83 upstream symbols, central
  event pipeline). Diff is additive-only: one field fill on one event type,
  no control-flow change; full runner regression suite gates it.

# ---8<--- flowpilot:change-ledger
feature_key: token-usage
source_doc_id: Task-440
change_type: feature
summary: fill ModelContextWindow from provider catalog in emitLocked when the adapter does not self-report — uniform field for TUI/desktop/CP-86 pressure consumers; pure in-memory, self-reported never overridden
# --->8---
