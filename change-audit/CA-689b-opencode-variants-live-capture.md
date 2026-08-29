# CA-689b — Opencode reasoning variants are per-model, captured live from ACP

## Problem

Operator report (CP-57 test guide section A): FlowPilot always showed the FULL
reasoning list for every opencode model, while the real opencode UI shows
per-model variants — `opencode-go/hy3` offers only `Default / none / low / high`
(screenshot), not minimal/medium/xhigh/max.

Root cause: `detectOpencodeModels` hard-coded one guessed list
(`minimal/low/medium/high/xhigh`, default `medium`) for EVERY model, and the
`opencode models` CLI cannot report per-model variants (plain id list only, no
JSON flag). The only live source of per-model variant truth is the ACP
`session/set_config_option` response, whose `configOptions` echo the effort
select for the freshly-selected model.

Also blocking test A4: the desktop right-sidebar stack had no scroll, so the
opencode account card (tall stats text) was unreachable below the fold.

## Fix

- `opencode_adapter.go`: capture the effort select from every successful
  `session/set_config_option` (model) response via
  `opencodeEffortOptionsFromConfig` and surface it through a new
  `onVariantsCaptured` hook.
- `opencode_variants_cache.go` (new): `recordOpencodeModelVariants` persists
  observed {model → efforts, default} to
  `<config>/FlowPilot/opencode_variants_cache.json` (+ in-memory overlay);
  `mergeOpencodeVariantOverrides` stamps observed truth over the detected
  list.
- `detectOpencodeModels`: the variant overrides apply on EVERY return path —
  the models cache stores the pre-capture guessed list, and the original patch
  only merged on the CLI/parse path, so a fresh cache hit hid the live truth
  (operator report "chưa thấy reasoning update"; live-verified: /providers now
  serves hy3 = [none, low, high] default none while unused models keep the
  guess).
- Registry wiring: opencode adapters write captures to the runner recorder;
  `detectOpencodeModels` merges overrides after the CLI list. The catalog
  becomes truthful per-model as models are actually used.
- Desktop `styles.css`: `.right-sidebar-stack` scrolls (`height:100%;
  overflow-y:auto`) — accounts cards below the fold are reachable again.
- Test guide section A statuses recorded (S + A1/A2/A3 passed; A2 requires the
  CA-689 DB migration first; A4 unblocked by the CSS fix).

## Tests

- `opencode_variants_cache_test.go` — fake ACP set_config response with
  default/none/low/high → capture hook fires with the exact list; merge
  overrides the guessed list + default; disk cache written; malformed payloads
  degrade to empty.
- Existing opencode suites green (capture is additive; SendTurn flow
  unchanged when no hook/payload).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Opencode per-model reasoning variants captured live from ACP config options into a variants cache, plus desktop sidebar scroll fix
# --->8---
