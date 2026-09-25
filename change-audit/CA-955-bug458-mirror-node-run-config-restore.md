---
id: CA-955
title: BUG-458 mirrored flow nodes regain run/posture/context_profile/config
type: bugfix
status: done
date: 2026-09-24
---

## Summary

`recordFromWorkflowRow` rebuilt FlowNode from step_definitions columns only,
silently dropping `run`, `posture`, `context_profile`, `config`, and pack-declared
`model` — the first four have no column; `model`'s column is admin-reserved
and sync never writes pack values. run-16693's tournament stalled after problem_scout because the
behaviorless `parallel_rollout` marker needs `run:inline` for the rollout
passthrough. Builtin mirrors now restore those fields from the embedded pack
(the authoritative source they were synced from; no admin-editable column
exists); all other rows derive `run` from behavior (`agent.*` -> delegate,
else inline).

## Files

- `internal/runner/supabase_workflow_flow_store.go` — post-reconstruction
  restore + derive pass; new `embeddedFlowNodesByID` helper.
- `internal/runner/bug458_mirror_node_fields_test.go` — 3 additive tests
  (RED before fix).

## Verify

- `go test -run TestRecordFromWorkflowRow` green; BUG-458 tests RED on HEAD.
- Live: run-16693 diag `flow_advance_target_not_spawnable` at
  parallel_rollout; Supabase row confirmed `behavior_id=null`, no run field.
