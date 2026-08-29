# CA-686 — Reasoning loads with the posture pin and the TUI accepts the full effort vocabulary

## Problem

Operator report during the CP-57 test run: "tôi chọn muse spark của opencode, nó
có reasoning xhigh, ở đây không show" — the scan posture pins
`opencode-go/muse-spark-1.2-contributor` + `reasoningEffort: xhigh`, but after a
restart the TUI showed the stale prefs value (medium), and `/reasoning xhigh`
was outright rejected ("Reasoning effort must be high, medium, or low") because
the TUI vocabulary was hard-coded to high/medium/low while the runner's
`opencodeReasoningVariantID` mapper supports minimal/low/medium/high/xhigh/max
and the model catalog advertises `supported_reasoning_efforts` (Task-215).

## Change

- TUI vocabulary: `reasoningEffortOptions = minimal, low, medium, high, xhigh,
  max`. **Model-first (operator follow-up: "không phải model nào cũng có xhigh,
  claude mới có max" — Desktop Task-215 parity):** when the selected model
  advertises efforts, that list IS the menu and the ONLY valid set — claude
  (low/medium/high/max) rejects xhigh, opencode muse-spark
  (minimal..xhigh) rejects max; empty stays "model default". Without catalog
  data the canonical vocabulary applies. `/reasoning` errors now list the
  model's actual options; the picker offers no unsupported value.
- CA-685 completion (operator decision 2026-08-29, supersedes CA-638/CA-641's
  reasoning-keep carve-outs): restart restore and `/new` re-apply in
  scan/plan/code reload the FULL profile — reasoning pin included. The `non`
  posture applies nothing, so a personal reasoning choice survives restarts
  there. `applyChatPostureProfile` drops the `keepReasoning` parameter.

- **Reasoning is dynamic per model end-to-end** (operator follow-up): switching
  models clamps a stale effort to the new model's catalog default — TUI
  `clampReasoningForCurrentModel` wired into `/model`, `/provider`, catalog
  fill and both posture paths; Desktop `setSelectedModel` parity. No catalog
  data → no clamp (cannot know).

- **Picker presentation + robust lookup (operator report: grok-4.5 picker
  showed the fallback vocab with bare "effort" details):** the /reasoning rows
  now read like the official grok CLI — human label + description per effort
  (`reasoningEffortDetail`: "Low effort — quick, fast implementations", …) with
  an "— active" marker on the current row; `modelReasoningEfforts` gained a
  cross-provider fallback so a model id resolves even when the stored provider
  key no longer matches the catalog shard. Wire-shape test locks grok-4.5 to
  exactly [high, medium, low] (its real /providers payload) — no minimal/xhigh/max.

## Tests

- `ca685_posture_non_mode_test.go` — operator's exact case (scan pin
  muse-spark + xhigh reloads both after restart); `/reasoning xhigh|minimal`
  accepted, unknown rejected.
- `picker_model_reasoning_test.go` — new vocabulary, "hi" matches high+xhigh,
  model-advertised list is the exact menu (claude list never offers xhigh),
  Enter accepts high (index 3 in the new list).
- `ca685_posture_non_mode_test.go` — `TestReasoningModelFirstValidation`:
  claude accepts max / rejects xhigh, opencode accepts xhigh / rejects max,
  picker lists exactly the model's efforts.
- `ca638_reasoning_restore_test.go` + `ca641_reasoning_new_preserves_choice_test.go`
  rewritten to the pin-applies spec (superseded operator decision, flagged per
  safe-fix R1; the "non" posture now covers the keep-my-choice use case).
- Old grok/gemini quota + posture suites green; full tui/app suite shows only
  the 7 documented pre-existing render failures.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-304
change_type: bugfix
summary: TUI reasoning accepts the full effort vocabulary and posture restore reloads the reasoning pin per CA-685 pin semantics
# --->8---
