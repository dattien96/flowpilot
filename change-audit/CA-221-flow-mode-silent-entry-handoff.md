# CA-221: Keep Flow Mode Entry Handoff Silent On The Hub

## Summary

Fixed a Flow Mode review-loop regression where starting a flow spawned the coder child but also sent the hub's first provider turn. In live use, that allowed a main-agent response to appear while the coder was the agent actually doing the work. The first flowRef turn now records the user turn, starts the flow entry child, and releases the hub without calling the parent provider; the hub is only reinvoked when the flow reaches its hub.inline synthesis node.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: first-turn `flowRef` handling now marks the turn as a silent flow handoff instead of prepending a wait prompt and running the hub provider.
- `apps/local-runner/internal/runner/flow_executor_test.go`: replaced the old wait-notice assertion with a regression test proving the child provider turn runs while the parent provider is not called.

## Verification

- `go test ./internal/runner -run 'TestStartTurnWithFlowRef|TestStartResolvedFlow|TestContinueReinvokeUsesEdgeResolvedTargetForFlowStartedRun'` passes (9 tests).
- Broader flow/review subset: 182 passed, 1 unrelated failure in `TestFlowEventsPathTraversalRejected` for Windows-style path traversal rejection.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-42
change_type: bugfix
summary: Suppress the hub's first provider turn for flowRef starts so Flow Mode review-loop output stays with the spawned coder child until hub synthesis is reinvoked
# --->8---
