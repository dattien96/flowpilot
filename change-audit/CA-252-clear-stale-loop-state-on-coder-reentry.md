# CA-252: Clear Stale Loop State On Coder Reentry

## Scope

Fixed a CP-36 Scenario 9 live regression from `run-13821`: after the hub submitted `continue`, coder reentry output appeared in both the coder transcript and the main hub transcript because stale loop status prevented auto-advance and routed coder output through the hub fallback path.

## Changes

- `interactive_service.go`: `applyFlowControl("continue")` now always sets a non-cap loop round to `running` and clears stale gate/block metadata, including prior `rejected` verdict state.
- `interactive_service.go`: `turnBridge.SubmitFlowControl` now rejects a second flow-control submission from the same provider turn, preventing a stale `escalate` from blocking a coder round that was already started by `continue`.
- `interactive_service_test.go`: added regression tests for `rejected -> continue -> running` and duplicate flow-control rejection.

## Verification

- `rtk go test ./internal/runner -run 'TestApplyFlowControl(LoopingResetsStepsSynchronously|ContinueClearsRejectedLoopStatus|CapReachedSettlesHubToWaitingUser)|TestSubmitFlowControlRejectsDuplicateSubmissionForSameTurn|TestCohortFailedMemberIncludedInNote|TestMarkFlowRunComplete(SettlesEveryStep|PreservesFailedAndSkipsUnstartedSteps)' -count=1` from `apps/local-runner` — passed.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-255
change_type: bugfix
summary: clear stale loop verdict state on continue and reject duplicate flow-control submissions from one turn
# --->8---
