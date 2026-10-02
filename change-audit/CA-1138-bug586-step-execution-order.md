# CA-1138 — BUG-586: reseed reordered the step timeline by topology, not execution

## What changed

`apps/local-runner/internal/runner/flow_step_runtime.go` (`mergeReseedSteps`):

- The BUG-562 merge built the list as `fresh-topology rows` + appended
  orphans — so a sprint reseed rendered its nodes ABOVE the ingest chain
  that ran before it (live run-139670: ten sprint steps over four
  cp_* rows).
- Now iterates `existing` first: each prior row keeps its first-seen
  position (with the merged status when still in the new topology, or as the
  historical row when orphaned); genuinely new nodes append in topology
  order. Status/timestamp merge semantics unchanged (BUG-562), duplicate
  NodeIDs deduped.

## Invariant

The step timeline is an execution log: first-activated order wins over the
latest mounted topology's index order.

## Tests

`bug586_step_execution_order_test.go` — red-first: ingest nodes reseeded
then sprint reseeded → every ingest row precedes every sprint row, DONE
statuses preserved.
