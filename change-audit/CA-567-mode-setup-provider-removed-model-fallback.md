# CA-567: /mode-setup remove provider field and model fallback to session

## What

Fixes still-empty model picker and removes `provider` from `/mode-setup` field picker per user rule (pick model → provider auto-pinned).

## Why

- Screenshot still showed `no models — set /provider first` for `/mode-setup scan model` even after CA-566 all-providers list. Live TUI has `providers[].Models == []` (GET /providers) while statusline shows `grok-4.5` from prefs. Picker had no fallback to `m.model`, so empty.
- User requested drop `scan provider` option; field list showed 5 items including `provider` (screenshot). Provider pin is now auto-inferred from model, so advertising it is redundant; typing it still works.

## Fix

- `apps/local-runner/internal/tui/app/helpers.go:1079,1100` — `filterModeSetupSuggestions` now takes `currentModel`; field picker shows `model, reasoning, yolo, clear` (no `provider`). Exact `provider` still handled in value stage for manual typing.
- `modeSetupValueSuggestions` `model` case: after collecting all `providers[].Models`, also appends `currentModel` if not already seen (with `currentProvider` as detail) so picker never empty when session has a model (live `grok-4.5`).
- `apps/local-runner/internal/tui/app/app.go:2078,2097` — call passes `m.model`; placeholder for `model` now `loading models…` / `no models in catalog` (not `set /provider first`); `provider` placeholder kept for manual typing.
- `chat_posture.go:212` — `model` edit already infers provider via `providerForModel` (CA-566), so removing provider field is safe.

Provider-agnostic: model/provider strings.

## Tests

- Updated `TestModeSetupPicker_PostureFieldValue` to assert `scan provider` not in field picker but `scan provider codex` still works via typed value stage.
- Existing `TestModeSetupModelShowsAllProviders` adjusted to set `m.model=""` and allow `>=3`; new fallback covered by `TestModeSetupModelInfersProvider` + manual check: empty catalog + `m.model=grok-4.5` → 1 row `scan model grok-4.5`.
- Full `go test ./internal/tui/app` green.

## Residual

- `provider` can still be pinned via `/mode-setup <p> provider <key>` by typing; not advertised.
- Desktop modal still shows provider dropdown (out of scope for this TUI picker change).

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: remove provider from mode-setup field picker and make model picker fallback to session model so picker never empty
# --->8---
