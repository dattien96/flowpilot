# CA-566: /mode-setup saving hang and model lists all providers

## What

Fixes two UX bugs after `/mode-setup scan provider`:
- Setting provider shows `Posture scan updated (saving…)` and hangs — never shows `saved`, picker still shows `current · provider` stale.
- `/mode-setup scan model` shows `no models — set /provider first` even though other providers have models; FlowPilot rule is picking a model auto-pins its provider, like `/model` lists all models.

## Root cause

- **Hang:** `editChatPostureProfile` set `dirty` and added `saving…` banner, then `Update: chatPostureMsg` batched `PUT`. PUT result was a bare `chatPostureMsg` with `pending=""` and `dirty=false`, so `Update` returned no banner — `saving…` stayed forever. No `saved`/`failed` replacement.
- **Model list:** `modeSetupValueSuggestions` for `model` used `modelsForProvider(providers, currentProvider)` → only current session provider (grok, which had `Models:[]`) → empty → placeholder. `/mode-setup` should not require prior `/provider`; model → provider should be inferred from catalog.

## Fix

- `apps/local-runner/internal/tui/app/model.go:313` — new fields `chatPostureSaving` + `chatPostureSavingPosture` to track the in-flight PUT.
- `apps/local-runner/internal/tui/app/chat_posture.go:210` — `providerForModel()` helper; `editChatPostureProfile` `model` case now infers `prof.Provider` from catalog; sets `chatPostureSaving` and keeps `saving…` banner.
- `apps/local-runner/internal/tui/app/app.go:265` — `chatPostureMsg` now clears `saving` on both success (adds `Posture <p> saved.` and updates `chatPostureCfg`) and error (adds `Posture <p> save failed: …`).
- `apps/local-runner/internal/tui/app/helpers.go:1229` — `model` picker now iterates **all** providers' models (dedup), detail = provider key, query matches id or provider. Empty catalog still shows placeholder, but grok-empty no longer hides claude/codex models.
- No runner/Desktop change — store already persists `provider`+`model` strings; TUI now sends inferred provider.

Provider-agnostic: provider/model strings, same for claude/codex/grok.

## Tests (additive only)

- `chat_posture_provider_fallback_test.go` additions:
  - `TestModeSetupModelShowsAllProviders` — grok empty, still lists opus/sonnet/o3 with provider detail.
  - `TestModeSetupModelInfersProvider` — `opus` → `Provider=claude` and `saving` set.
  - `TestModeSetupSavingReplacedWithSaved` — `saving` → PUT success → flag cleared and `Posture scan saved.` in View.
- Old suite stays green: `go test ./internal/tui/app` (all), `TestModeSetupPicker_PostureFieldValue` etc.

## Residual

- `model` placeholder still `no models — set /provider first` when catalog has zero models total — not synthesized.
- Desktop modal still has separate provider dropdown + model input; auto-infer could be added there if needed.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: fix mode-setup saving hang (saved/failed banner) and make model picker list all providers with provider auto-inferred
# --->8---
