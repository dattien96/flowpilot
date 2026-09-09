# CA-800 — Flow statusline shows bare vibe-sprint id

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Flow chrome StatusLabel strips pack prefix so the bar shows vibe-sprint not flowpilot-core-flow-p…
# --->8---

## Why

Live `/open run-220036`: infer already set `vibe-sprint`, but the input-frame title used the full pack ref and truncated at 24 runes to `flowpilot-core-flow-p…`.

## Change

`LaunchArm.StatusLabel` runs `workingmode.BareFlowID` on Label/FlowRef/WorkflowID/StepID.

## Tests

New `ca800_flow_statusline_bare_id_test.go`: pack ref → `vibe-sprint`; catalog human name unchanged; title has `vibe-sprint` not truncated pack. Old statusline tests untouched.

## Providers

Agnostic Case 1: TUI label helper takes no `providerKey`.

## Will not undo

CA-799 reconstruct infer + chat-kind step restore. Catalog names like `Task Harness` stay.
