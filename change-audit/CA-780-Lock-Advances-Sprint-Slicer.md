# CA-780 — After SS lock, dispatch sprint_slicer hub.inline

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: tryAdvanceFlowThroughInline handles hub.inline; resumeVibeLock advances ss_lock --done--> sprint_slicer synchronously
# --->8---

## Why

Live run-213333: [Retry]/continue marked `ss_lock` DONE but `sprint_slicer` stayed pending while hub `Thinking`. `hub.inline` was not in `flowNodeInlineDispatchable`, so advance returned false.

## Change

- `hub.inline` dispatchable; reuse `dispatchHubNotifyNode`
- `resumeVibeLock` calls `tryAdvanceFlowFromNode` synchronously

## Tests

`ca780_ss_lock_advances_slicer_test.go`

## Will not undo

CA-777 park ss_lock. CA-779 [Lock] label.
