# CA-788 — cp_writer terminal done must not repark ss_lock

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: claim cp_writer/task_slicer --done--> done so advanceOrNotifyHub does not reinvoke the SS/CP validator hub and park the lock a second time
# --->8---

## Why

Live run-214743: first `ss_lock` → `cp_writer` DONE, then hub synthesizer `REQUEST CHANGES` → `ss_converter` → second `ss_lock`.

`cp_writer --done--> done` is a terminal pseudo-node (not in `activeFlowNodes`). `tryAdvanceFlowFromNode` returned false → `advanceOrNotifyHub` reinvoked `ss_validator` → `changes_requested` resolved the `ss_lock --continue--> ss_converter` back-edge.

## Change

`tryAdvanceFlowFromNode`: when the sole forward-done target is `done` and the completed node is `cp_writer` or `task_slicer`, mark the node DONE and return true. `onVibeCpNodeDone` still chains vibe-cp-ingest / sprint.

## Tests

`ca788_cp_writer_terminal_no_ss_lock_test.go` — claimed terminal; `ss_lock` stays not WAITING. Old tests untouched.

## Providers

Agnostic: no `providerKey` branch. Edge claim is topology-only.

## Will not undo

CA-786 ingest→CP chain. CA-777 hub done parks first ss_lock. CA-787 inherit.
