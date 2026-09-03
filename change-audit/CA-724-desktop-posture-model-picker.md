# CA-724 — Desktop posture setup modal: model picker preload + provider auto-infer (TUI parity, E7 follow-up)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-348
change_type: feature
summary: ChatPostureSetupModal drops the manual provider select and free-text model input — Model is now a preloaded grouped picker (supported registry + runner-detected + current pin) that auto-infers the provider on pick, with catalog-gated reasoning; pure logic in postureModelPicker.ts covered by 10 node:test cases
# --->8---

## Problem

- Operator testing E7 flagged the Desktop posture setup modal: free-text
  Model input forces users to know exact model IDs, and a separate Provider
  select lets provider/model disagree. TUI `/mode-setup` has no provider
  picker — provider is always derived from the picked model.

## Changes

- `apps/desktop-flowpilot/src/components/postureModelPicker.ts` (new, React-free):
  `inferPostureProvider` (TUI `providerForModel`), `applyPostureModelPick`
  (TUI `applyModalPickerSelection` model kind — inherit clears all three
  fields), `buildPostureModelGroups` (TUI `allModelsAcrossProviders` —
  enabled registry + detected + current-pin fallback), `postureReasoningOptions`
  (TUI `reasoningOptionsForModel` + `isReasoningEnabled`), default list mirrors
  TUI `reasoningEffortOptions` (incl. `minimal`, which the old modal lacked).
- `ChatPosturePanel.tsx` (`ChatPostureSetupModal`): removed Provider select;
  Model is a `<select>` with per-provider `<optgroup>`; read-only
  `Provider auto: <key>` line; Reasoning select driven by the picked model's
  catalog efforts and disabled until a model is picked (keeps the current
  value as an extra option when outside the list).
- `postureModelPicker.test.ts` (new): 10/10 green via standalone `node --test`.

## Verification

- `node --test /tmp/bug348-test/postureModelPicker.test.js`: 10 pass, 0 fail.
- Project `tsc --noEmit`: no new errors (single error in
  `store.chat-mode-persist.test.ts` is pre-existing — confirmed via stash).
- Desktop live retest (E7 with the new picker): pending operator.
