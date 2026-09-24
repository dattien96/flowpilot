# CA-965 — Devin thought_level reasoning schema + stale-model disable on Detect (Task-438)

## Summary

Devin CLI ≥3000.11.x changed the ACP session schema: model IDs are bare
live-catalog IDs (`swe-2-high`, `claude-opus-4-7-medium`, …) and reasoning is
a separate config option `thought_level` (`medium|high|max`). FlowPilot was
still concatenating effort into the model ID (`swe-2-max`), which the live
catalog rejects with `Invalid value` — the reasoning picker had silently
become a no-op for Devin.

1. **New-schema reasoning path** — `session/new` config options are now
   harvested for `thought_level`; when advertised, the adapter sends the
   catalog-valid bare model ID plus a separate
   `session/set_config_option{configId:"thought_level"}` mapped from the
   FlowPilot reasoning effort (`devinThoughtLevelForEffort`: canonical alias
   map, exact match first, nearest-level fallback rounding equidistant picks
   upward so `xhigh` → `max`, never under-thinks). When `thought_level` is
   absent (older CLI), the legacy effort-suffixed model remap is unchanged.

2. **Stale model fallback** — `devinCatalogModelFor` resolves a requested
   model against the captured live catalog: exact match wins, effort-suffixed
   IDs fall back to a same-family catalog entry (`swe-2-max` → `swe-2-high`),
   unknown IDs pass through so provider errors stay honest.

3. **Detect Model staleness** — the probe harvests `thought_level` values as
   each detected model's `supported_reasoning_efforts`
   (`devinCatalogChoicesToProviderModelsWithEfforts`,
   `recordDevinModelCatalogWithEfforts` — additive variants; the pre-existing
   signatures used by green tests are unchanged). Desktop `detectModels` now
   disables previously-detected rows that are absent from the live catalog
   (`staleDetectedModelRows`, `isEnabled:false` — never deletes, never touches
   manually-added rows, preserves user edits) and reports the stale count in
   the summary.

## Changes

- `apps/local-runner/internal/runner/devin_reasoning.go` —
  `devinThoughtLevelForEffort`, `devinCatalogModelFor`, family/effort parsing.
- `apps/local-runner/internal/runner/devin_adapter.go` — capture
  `thought_level` options on the adapter; `applyDevinSessionConfig` emits
  `thought_level` on new-schema sessions, legacy remap otherwise.
- `apps/local-runner/internal/runner/devin_models_probe.go` — harvest
  thought-level values into `SupportedReasoningEfforts`.
- `apps/local-runner/internal/runner/devin_models_cache.go` — additive
  effort-aware variants of the catalog→model mapping + record functions.
- `apps/local-runner/internal/runner/devin_thought_level_test.go` — new
  (6 tests: mapping, catalog fallback, config emission, probe harvest).
- `apps/local-runner/internal/runner/testdata/devin_acp/session_new_thought_level_result.json`
  — new fixture mirroring the 3000.11.3 live capture.
- `apps/desktop-flowpilot/src/components/settings/aiProvidersDetect.ts` —
  new `staleDetectedModelRows` helper.
- `apps/desktop-flowpilot/src/components/settings/aiProvidersDetect.test.ts`
  — new (4 tests).
- `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx` —
  Detect flow disables stale detected rows + summary line.

## Tests

- `go test ./internal/runner -run 'ThoughtLevel|CatalogModel|Devin'` — green
  (new tests red-first by assertion, then implemented).
- `node --test` on `aiProvidersDetect.test.js` — 4/4 green.
- `tsc --noEmit` (desktop) — clean.
- Provider parity: change is confined to the devin adapter/probe seam;
  `ProviderEvent`/session contracts untouched. Claude/Codex/Grok paths
  unchanged (their effort mapping is not consulted on the devin path).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-438
change_type: feature
# --->8---
