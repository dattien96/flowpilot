# CA-1210 — markFlowRunComplete scoped to the active flow mount (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04): when sprint-042's audit emitted
`flow_control: done` at 20:16:49, the terminal cascade stamped
`task_plan_reader` — a node of the PARENT vibe-tasks graph — DONE, even
though that flow's lifecycle was nowhere near over (4 tasks remained).

## Root cause
`markFlowRunComplete` iterated every step row of the runID. One run hosts
rows from every mounted flow that ever seeded it (BUG-562 merge semantics
preserve foreign rows on reseed), so the sealing flow's terminal cascade
reached across mounts and stamped rows belonging to the still-running
parent graph.

## Fix
The cascade is scoped to the run's currently-active flow node set
(`activeFlowNodesFor`) — the flow whose hub actually emitted `done`.
Foreign-mount rows are skipped entirely. An empty active node set keeps
the legacy stamp-everything behavior (defense for non-graph mounts).

Orphan leg cleanup is unaffected — `reconcileChildRunsOnFlowDone` still
settles orphaned child runs; only the step-row stamping is scoped.

## Tests
- `ca1210_flow_complete_scoped_to_active_mount_test.go`
  `TestCA1210_FlowCompleteLeavesForeignMountRowsUntouched` — sprint-graph
  rows settle DONE/SKIPPED while a foreign `task_plan_reader` row stays
  PENDING. RED pre-fix (row was SKIPPED), green post-fix.
