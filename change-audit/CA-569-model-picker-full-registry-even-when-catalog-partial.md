# CA-569: Model pickers show full registry even when catalog partial

## What

Fixes `/model` and `/mode-setup model` still showing only grok when other providers' `Models` are empty or providers missing from `m.providers`.

## Why

CA-568 added `allModelsAcrossProviders` that unioned `providers[].Models` + defaults for present providers with empty `Models` + `currentModel`. Live TUI had `m.providers` containing only grok (or single provider in test) → other providers absent → their defaults never added → picker showed only grok, user suspected `Available` filtering. Actual cause: missing-provider case not covered; `Available` was never filtered (intentionally).

## Fix

- `apps/local-runner/internal/tui/app/helpers.go:1430` — `allModelsAcrossProviders` now also appends registry defaults for known keys (`claude, codex, grok, gemini`) not present in `m.providers` at all, so even single-provider catalog or empty catalog yields full list. Keeps dedup by id, provider detail, and `currentModel` fallback.

Provider-agnostic: static registry maps directly to runner `providerSpecs`.

## Tests

- `go test ./internal/tui/app` still green; `TestModelTabCompletesSelection` still passes (q filtered by id only, first match remains `o3` even with full list). `TestModeSetupModelShowsAllProviders` now `>=3` allows extra gemini defaults.

## Residual

- Picker shows `no models in catalog` only when `all` empty (no catalog, no currentModel) — rare.
- `Available`/`Installed` still not used to hide models; intentional per full-list requirement.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: ensure model pickers include defaults for missing providers so full supported list shows even when catalog partial
# --->8---
