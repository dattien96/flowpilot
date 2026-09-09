# CA-797 — mid-graph flow join marks skipped predecessors

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: startResolvedFlowFromNode marks forward-done predecessors SKIPPED so the TUI sidebar is not empty [ ] for nodes the join never ran
# --->8---

## Why

Live run-220036: CA-791 overlay joined vibe-cp-ingest at `task_slicer`. Sidebar showed `cp_reader` / `cp_validator` / `cp_lock` as pending `[ ]` while `task_slicer` was RUNNING. `reseedFlowStepRuntime` seeds every overlay node PENDING; skipped prefix never got a status.

## Change

After reseed, when `startNodeID` is set, stamp every forward-`done` predecessor SKIPPED (`[-]` in TUI). User-start (empty start node) unchanged — `cp_lock` still runs.

## Tests

New `ca797_join_skips_predecessor_steps_test.go`. Old CA-791 tests untouched.

## Providers

Agnostic: step status + edge walk take no `providerKey`.

## Will not undo

CA-791 join at `task_slicer` (no `cp_reader` spawn). CA-796 owner-fail settle.
