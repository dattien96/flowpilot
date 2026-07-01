# CA-163: Forward-Edge Auto-Spawn of the Reviewer Cohort (Task-180 Follow-Up)

## Scope

Closes the gap CA-161 mistakenly assumed didn't exist: forward flow progression (coder → reviewer cohort) was not actually AI-decided from any topology information — nothing told the hub's reasoning what nodes to spawn next. This was surfaced by a direct user question ("how could the AI possibly know the node/edge topology we defined, in order to forward?"), which prompted re-investigation of the actual call graph rather than trusting the earlier (incorrect) assumption recorded in CA-161.

## What was actually true before this change

- The "Coder completed" pending-context note was generic text with no topology information.
- `spawn_agent`'s tool description does not enumerate a flow's nodes/edges.
- No existing E2E test exercised an AI actually deciding to spawn the reviewer cohort — all prior tests spawned it manually, to test the Go-side join/cap logic in isolation.

So forward progression had no real driver at all for flowRef-started runs; it happened to "work" only when a human or test manually spawned the next step.

## Fix

`tryAdvanceFlowFromNode(parentRunID, completedNodeID, resultMessage)` (`flow_executor.go`): when a tracked node completes, resolves its forward (`Kind: "forward", When: "done"`) edges via `forwardDoneTargets`, validates every target is a spawnable `agent.delegate` node (bailing — returning `false` — for anything else, e.g. `synthesis`'s `hub.inline`), and spawns all matching targets as a single shared cohort (`FlowCohortID`, `CohortSize`) reusing the existing CP-36 join/cap machinery. Wired into the `EventTurnCompleted` handler via `advanceOrNotifyHub`, which tries this deterministic path first and only falls back to the legacy note+`maybeAutoReinvokeHub` behavior when the run has no tracked flow edges (i.e. an AI-initiated flow, not one started via a resolved `flowRef`).

The `synthesis` node transition (`hub.inline`) is intentionally left alone: it's the hub's own reinvoke turn, not a spawned child, and was already correctly Go-orchestrated by the pre-existing cohort-join → `maybeAutoReinvokeHub` path.

## Verification

- `TestCoderCompletionAutoSpawnsReviewerCohort` (`flow_executor_test.go`): genuine end-to-end proof — starts a flow, lets the coder's turn complete via a fake adapter, and asserts Go (not any AI/test-code decision) spawns exactly `reviewer_correctness` and `reviewer_security` in one shared cohort.
- `TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget`: proves the safety valve for non-delegate targets (e.g. `synthesis`).
- `TestForwardDoneTargetsFindsFanOutTargets` / `...NoMatchReturnsEmpty`, `TestFindFlowNodeReturnsMatchByID`: unit coverage of the edge/node resolution helpers.

## Correction to CA-161

CA-161's "Still open" section previously stated forward progression "remain[ed] AI-decision-driven, not edge-data-driven" as a deliberate, correct scoping choice. That was wrong, not a scoping choice — see above. CA-161 has been amended to point here instead of restating the incorrect claim (closes BUG-NOTE-CP42 #34, alongside this note).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-180
change_type: feature
summary: Go deterministically auto-spawns the forward-edge reviewer cohort from a flow's own edges_json when a tracked node completes, closing the gap where forward progression had no real driver at all
# --->8---
