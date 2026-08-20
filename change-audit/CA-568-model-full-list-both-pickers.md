# CA-568: /model and /mode-setup model show full supported list

## What

Fixes both pickers showing only current provider's models (grok cache) instead of full supported list. User reported `/model` and `/mode-setup scan model` only showed grok models, missing claude/codex/gemini.

## Root cause

- `/model` picker `app.go:2062` used `modelsForProvider(m.providers, m.provider)` → only grok.
- `/mode-setup` model `helpers.go:1245` iterated `p.Models` directly → grok cache hit, claude/codex/gemini `Models=[]` (no detect) → empty.
- `TestModelTabCompletesSelection` locked single-provider behavior, so naive union broke it (provider name contains query).

## Fix

- `apps/local-runner/internal/tui/app/helpers.go:1420` — new `defaultModelsForKey` (mirrors runner/providerSpecs) and `allModelsAcrossProviders(providers, currentProvider, currentModel)`:
  1. All `p.Models` (dedup),
  2. Providers present but `Models` empty → registry defaults (codex 3, claude 3, grok 2, gemini 5),
  3. `currentModel` appended if not seen (live `grok-4.5` from prefs while Models=[]).
  Returned as `[]modelEntry{provider, id}`.

- `helpers.go:1234` — `/mode-setup` `model` case now uses `allModelsAcrossProviders`, detail = provider, query matches id or provider.

- `apps/local-runner/internal/tui/app/app.go:2062` — `/model` picker now uses same helper, detail `provider` (+ `· current`), query matches id only (keeps `TestModelTabCompletesSelection` green). Empty catalog → `loading models…` / `no models in catalog`.
- `app.go:3129` — `/model <name>` now accepts any id from full list, auto-switches `m.provider` via `matched.provider` or `providerForModel`, binds account, clears `skillsCatalog`, `cmdLoadSkills` if switched. `/model` list display now grouped `provider → model` with `*` current.

Provider-agnostic: model/provider strings.

## Tests

- Existing `TestFilterModelSuggestions`, `TestEnter_AcceptsHighlightedModelSuggestion`, `TestModelTabCompletesSelection` still pass (single-provider case unchanged).
- `TestModeSetupModelShowsAllProviders` now expects `m.model=""` and `>=3`; prior `TestModeSetupProviderFallback` still pass.
- Full `go test ./internal/tui/app` green.

## Residual

- `/mode-setup` provider field already removed in CA-567; `/model` keeps provider detail for visibility.
- Defaults are static registry copies; live detect (codex debug, grok cache) still preferred when present.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: make /model and mode-setup model list every supported model across providers with defaults and auto-switch provider on pick
# --->8---
