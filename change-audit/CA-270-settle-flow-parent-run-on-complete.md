# CA-270: Settle Flow Parent Run On Complete

## Summary

Fixed a flow-engine terminal-state bug where a parent flow run could remain `waiting_question` after the flow had already reached `loop_state.status="done"`.

## What Changed

- `markFlowRunComplete` now settles the parent `interactiveRun` to `completed` when the flow terminal transition runs.
- Pending approval/question gates on that parent are cleared and marked expired so reopening history cannot replay an already-obsolete question card.
- Resume normalization now treats persisted flow records with `loop_state.status="done"` as completed even if an older persisted run status still says `waiting_question`.

## Why

`run-15046` showed the failure mode: the final persisted session row had `status:"waiting_question"` and `loop_state.status:"done"`. While the runner was still alive, switching history could resurrect the stale question form. After restart, `waiting_question` normalized to `cancelled`, so the left history bar showed `cancelled` even though the flow had completed.

## Validation

- `rtk go test ./internal/runner -run 'Test(MarkFlowRunCompleteSettlesParentWaitingQuestion|ProjectRunHistoryKeepsCompletedStatusWhenReopeningLegacyFlowRun|ReconstructCompletedAutoFlowWithoutLoopStateKeepsCompleted|ReconstructIncompleteCompletedFlowRunCancelsResumeAndDisablesAutoOrchestrate)' -count=1`
- `rtk go test ./internal/runner -run 'Test(MarkFlowRunComplete|E2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes|E2EReviewLoopMultiRoundChangesThenApprovedCompletes|Normalize|Reconstruct)' -count=1`
- Full `rtk go test ./internal/runner -count=1` ran with `1253 passed`, `11 skipped`, and one temp-dir cleanup failure in `TestE2ECodingRetryReusesSamePackageID`; no assertion failure from this change.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-36
change_type: bugfix
summary: Settled flow parent runs and stale question gates when flow completion reaches loop_state done.
# --->8---
