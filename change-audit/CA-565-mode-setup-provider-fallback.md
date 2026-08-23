# CA-565: /mode-setup provider picker fallback when catalog empty

## What

Fixes `/mode-setup scan provider` showing `(no providers)` (screenshot gate-sandbox master) when `m.providers` is empty (catalog not yet loaded). Pins are plain strings — user must be able to pick `claude/codex/grok` even before provider install.

## Root cause

- `filterModeSetupSuggestions` → `modeSetupValueSuggestions` provider case only iterated `providers []client.Provider`. Empty catalog → `[]` → `collectSuggestions` fell through to placeholder `"(no providers)"` (`app.go:2072`).
- No fallback to `providerAccounts` (already authenticated) or known keys. `/provider` picker already had `loading providers…` branch; `/mode-setup` treated empty as terminal.
- Reporter `agent` context had `gate-sandbox` project but providers not loaded, so picker was always empty.

Provider-agnostic: provider keys are plain strings, same for claude/codex/grok.

## Fix

- `apps/local-runner/internal/tui/app/helpers.go:1076,1165` — `filterModeSetupSuggestions` now takes `accounts []ProviderAccountSummary`; `modeSetupValueSuggestions` provider branch:
  1. keys from `providers`,
  2. if none, keys from `providerAccounts.ProviderKey`,
  3. if still none, `claude/codex/grok` fallback,
  de-duplicated, filtered by query, detail via `providerSuggestionDetail` when available else `"provider"/"current · provider"`.
- `apps/local-runner/internal/tui/app/app.go:2059,2062` — `collectSuggestions` passes `m.providerAccounts` to filter; placeholder for `provider` now shows `loading providers…` only while `!sessionDefaultsLoaded && len==0`, otherwise `"(no matching providers)"` for filtered-empty — the fallback makes the former empty unreachable for provider, but keeps UX consistent with `/provider`.
- No runner/Desktop change — runner store already persists `provider` string regardless of catalog.

## Tests (additive only)

- `chat_posture_provider_fallback_test.go`:
  - `TestModeSetupProviderFallbackWhenCatalogEmpty` — nil providers+accounts → 3 items claude/codex/grok.
  - `TestModeSetupProviderUsesAccountsWhenProvidersEmpty` — accounts `myprov,codex` → picker shows those.
  - `TestModeSetupProviderFiltersFallback` — `cla` → only `claude`.
- Old suite stays green: `go test ./internal/tui/app` (all), existing `TestModeSetupPicker_PostureFieldValue` still passes.

## Residual

- `model` picker still shows `no models — set /provider first` when `modelsForProvider` empty — not synthesized (models are provider-specific and catalog-driven).
- Desktop `ChatPosturePanel` uses `supportedModels` from catalog; same empty-model UX.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: mode-setup provider picker falls back to accounts and known claude/codex/grok when catalog empty so pin is always selectable
# --->8---
