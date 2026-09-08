# CA-789 — cp_writer is not a review cohort

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: spawn cp_writer/task_slicer without FlowCohortID; a writer cohort join advances the chain instead of reinvoking ss_validator (second ss_lock)
# --->8---

## Why

Live run-216140 (after CA-788): `ss_lock` → `cp_writer` DONE → `ss_validator` hub turn → `changes_requested` → `ss_converter` → `ss_lock` again (3 cycles).

CA-788 only claimed `advanceOrNotifyHub` when `flowCohortId` is empty. `tryAdvanceFlowFromNode` spawned `cp_writer` as a size-1 review cohort (`flow-auto-ss_lock-round-N`). Child complete joined that cohort and `maybeAutoReinvokeHubWithNote` revived `ss_validator`.

## Change

- Spawn: `vibeLinearWriterNode` (`cp_writer`, `task_slicer`) gets empty `FlowCohortID`.
- Join: if the completed cohort is only those writers, `advanceOrNotifyHub` (terminal claim) instead of hub reinvoke; do not stamp hub RUNNING.

## Tests

`ca789_cp_writer_not_review_cohort_test.go`. Old tests untouched.

## Providers

Agnostic: spawn/join has no `providerKey` branch.

## Will not undo

CA-788 terminal-done claim. CA-777 first ss_lock park. CA-786 ingest→CP chain.
