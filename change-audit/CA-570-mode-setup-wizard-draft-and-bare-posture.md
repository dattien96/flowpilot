# CA-570: /mode-setup wizard with draft and bare posture picker

## What

Fixes two UX issues:
1. Bare `/mode-setup` showed no `scan` to pick (required trailing space).
2. Each `/mode-setup <p> <field> <value>` saved immediately and exited — user had to retype for every field (model, reasoning...). Desired: pick multiple fields/postures then save once, with back navigation.

## Why

- `parseSlashArgPrefix` required space after `/mode-setup`, so bare command fell through to slash list, not posture picker. Field `provider` was still advertised then removed, but posture never shown without space.
- `mode-setup-value` Enter called `processInput` → `GET` + `edit` + `PUT` per field. No draft, no way to batch.
- No back mechanism (`← back`, Esc, Shift-Tab) to return from value → field → posture.

## Fix

- `apps/local-runner/internal/tui/app/helpers.go:1079` — bare `"/mode-setup"` (trimmed, case-insensitive) now returns posture picker (`scan|plan|code`) directly, before `parseSlashArgPrefix`. Posture → Tab still expands to `"/mode-setup <p> "`.
- `model.go:318` — new `modeSetupDraft *ChatPostureConfig` + `modeSetupDraftDirty` to stage wizard edits (deep copy of `chatPostureCfg`).
- `app.go:collectSuggestions` — wizard extra rows: at posture level (`/mode-setup`) when dirty, append `✓ save` / `✕ cancel`; at field (`/mode-setup plan `) and value (`/mode-setup plan model `) prepend `← back`.
- `app.go: Update Enter` — `mode-setup-value` now stages into `modeSetupDraft` (infer provider for model, handle yolo/reasoning/clear), shows `Staged <p> <field> = <v>` and returns to field picker (`/mode-setup <p> `) instead of immediate PUT. `mode-setup-save` does single `PUT` of draft, clears wizard, sets `chatPostureSaving`; `mode-setup-cancel` discards; `mode-setup-back` navigates up one level based on `parts` length.
- `app.go: KeyEscape` / `KeyShiftTab` — when input starts with `/mode-setup`, navigate back one level (or discard at root if dirty), mirroring `← back` row. `applySuggestion` (Tab) also handles wizard save/cancel/back.
- Field picker detail: `provider` removed from advertised list (model auto-pins), but typed `provider` still works for value stage.

## Tests (additive)

- `mode_setup_wizard_test.go`:
  - `TestModeSetupBareShowsPosture` — bare `/mode-setup` shows `scan`.
  - `TestModeSetupWizardStageAndSave` — stage `plan model opus` (provider inferred `claude`) + `plan reasoning high`, input returns to `/mode-setup plan`, then save shows `✓ save` and triggers PUT via `mode-setup-save`.
  - `TestModeSetupWizardBack` — value and field pickers contain `mode-setup-back`.
- Updated `chat_posture_provider_fallback_test.go` to filter out `mode-setup-back`/`save` rows when asserting provider counts.
- Full `go test ./internal/tui/app` green; `TestModeSetupPicker_PostureFieldValue` still passes (field picker no longer advertises `provider`).

## Residual

- Typed 3-arg command `/mode-setup <p> <field> <value>` still does immediate `GET`+`PUT` via `handleSlashCommand` (power-user path).
- Desktop modal unchanged (separate 3-tab UI).

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: make bare /mode-setup show posture picker and add wizard draft with back/save so multiple fields/postures can be staged then saved once
# --->8---
