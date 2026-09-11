# CA-793 — vibe checkpoint commits only when artifacts exist

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: feature
summary: Vibe history records last node only after workspace files exist; deleted SS/CP/Task demotes checkpoint on reconstruct
# --->8---

## Why

Stop / token / net must not replay ingest from scratch, but path-only persist lies if the user deleted files in the target project. History update is fail-closed: no `stat` + size>0 → do not move the checkpoint.

## Change

- `tryCommitVibeCheckpoint` after vibe node done: collect artifacts, `os.Stat` all, skip update if any missing or empty
- `normalizeVibeCheckpointLayer`: `audit`→`tdd`, `sprint_slicer`→`task_slicer`, `cp_lock`→`cp_writer`, `ss_validator`→`ss_lock` (review: alias miss skipped demote walk)
- `applyVibeCheckpointFromDisk` on reconstruct: demote `tdd → task_slicer → cp_writer → ss_lock → empty`

## Tests

New `ca793_vibe_checkpoint_files_test.go`. Old reconstruct tests untouched.

## Providers

Agnostic: checkpoint/`os.Stat` take no `providerKey`.

## Replay

Same-run reconstruct re-stats workspace. Does not yet auto-`startResolvedFlowFromNode` at the demoted layer (resume wiring next).

## Will not undo

CA-792 debate stash (RAM). CA-791 join. CA-770 awaiting-lock reconstruct.
