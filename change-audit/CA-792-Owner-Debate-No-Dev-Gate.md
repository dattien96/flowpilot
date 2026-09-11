# CA-792 — vibe-owner-debate must not show Dev escalate cards

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Owner debate copies empty acceptance, spawns both owners, skips review-cohort done gate, restores sprint graph (run-628898 Dev 1/2/3)
# --->8---

## Why

Live run-628898: after `tdd`, `vibe-owner-debate` overlaid the sprint. `startInlineEntryChain` spawned only `owner_1`, left sprint `acceptance_nodes` including `synthesis`, then `debate_synthesis` done hit `synthesisDoneVerdictError` (`cohort: review` empty vs `owner_debate`) and escalated to Dev Retry/Stop/Revise. V6 forbids Dev `1/2/3` in vibe.

## Change

- `startInlineEntryChain`: copy `AcceptanceNodes`; spawn every `agent.delegate` done-target (both owners) as one cohort
- `synthesisDoneVerdictError`: no-op when `cohort: owner_debate` is present and review cohort is empty
- Stash sprint graph before debate overlay; restore on `debate_synthesis` done

## Tests

New `ca792_owner_debate_no_dev_gate_test.go`. Old tests untouched.

## Providers

Agnostic: verdict/topology/stash take no `providerKey`.

## Will not undo

CA-791 ingest join at task_slicer. CA-788/789 writer terminal. CA-790 lock-once.
