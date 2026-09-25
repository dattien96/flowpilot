# BUG-431: Opencode default model resolves to Interactions-API-only `google/deep-research-max-preview` — every turn fails when model left empty

## Metadata

- Document ID: `BUG-431`
- Title: `model="" on multi-family opencode account resolves to google/deep-research-max-preview-04-2026 → "This model only supports Interactions API" every turn`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: evidence `~/fp-beds/lt-evidence/ui/RESULT.md` (BUG-LIVE-UI-2)
- Feature Keys: `opencode`, `model-resolution`, `provider-defaults`, `turn-dispatch`

## AI Quick View

### Summary

- When a turn runs with `model=""` on the multi-family `opencode` account, FlowPilot issues `session/set_config_option model=google/deep-research-max-preview-04-2026`.
- That model only supports the Interactions API → every turn fails with `Internal error: This model only supports Interactions API.` — the provider appears ready but cannot complete a single turn until a valid model is picked manually.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

### Symptom

An opencode turn launched without an explicit model (`model_override=<nil> resolved_model=""`) immediately fails: the ACP `session/set_config_option` selects `google/deep-research-max-preview-04-2026` and the provider returns `Internal error: This model only supports Interactions API.`

### Expected

An empty model resolves to a usable default for the account family (a chat-completions-capable model), or the account surface marks the model required before turn dispatch.

### Actual

Every `model=""` turn on the multi-family opencode account (Google·OpenCodeGo·xAI) fails identically. `runner.log`: `[turn] run_id=run-1 model_override=<nil> resolved_model=""` → `set_config_option …deep-research…` → `turn-failed`. Provider looks healthy in `/status`; only manual `/model` selection unblocks.

### Impact

Dead-by-default provider path: users on multi-family opencode accounts who never pick a model get a guaranteed turn failure with no guidance toward the fix.

## Reproduction

1. Connect a multi-family opencode account; leave model unset.
2. Start any chat turn.
3. Observe `session/set_config_option model=google/deep-research-max-preview-04-2026` → `Internal error: This model only supports Interactions API.` → turn failed.

## Root cause

- Default-model resolution for the opencode provider picks `google/deep-research-max-preview-04-2026` — an Interactions-API-only model — when `model=""` (resolution table in the opencode adapter/model catalog; the failure is deterministic on the multi-family account). Exact mapping location not yet isolated — capture only.

## Evidence

- `~/fp-beds/lt-evidence/ui/RESULT.md` — BUG-LIVE-UI-2: `runner.log` (`[turn] run_id=run-1 model_override=<nil> resolved_model=""`, `set_config_option …deep-research…`, `turn-failed`), `ui-drift-t1b.txt`.

## Severity

- `low` — deterministic turn failure on an unset-model path; trivially worked around by picking a model, but the default is guaranteed-broken.

## Completion Notes (implemented 2026-09-23, CA-917b)

- Fix: `opencodeIsChatCapableModel` filters non-conversational families
  (deep-research/Interactions-API-only, embedding, veo, lyria, tts, live-*,
  computer-use) from the opencode catalog in both verbose and plain
  `opencode models` parsers — same pattern as the existing
  deepseek-v4-flash-free skip. The picker can no longer default to
  google/deep-research-max-preview-04-2026.
- Note: run-1 evidence shows model="" already works (server default
  big-pickle). The dead-by-default path was the catalog offering the
  Interactions-only model as a selectable default → run.modelName stored it.
- Tests: `bug431_opencode_catalog_filter_test.go` (plain + verbose).
