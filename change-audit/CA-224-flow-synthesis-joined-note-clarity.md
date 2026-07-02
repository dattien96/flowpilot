# CA-224: Make Flow Synthesis Joined Notes Explicit

## Summary

Fixed a Flow Mode review-loop hang where both reviewer agents could complete but
the synthesis step still sat in `RUNNING`. The hub prompt did contain the
reviewer cohort note, but the note was wrapped inside a generic sub-agent
system-note block and blank reviewer outputs rendered as empty lines. The joined
note is now explicit about being the flow-engine synthesis input, and empty
reviewer outputs are surfaced as a placeholder instead of disappearing.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: updated
  `buildCohortNote` so the synthesized reviewer handoff begins with
  `[flow-engine joined result note]`, states that it is part of the current
  prompt context, and renders `(completed with no final message captured)` when a
  completed reviewer produced no final text.
- `apps/local-runner/internal/runner/interactive_service_test.go`: added
  regression coverage for the new joined-note marker and the empty-final-message
  placeholder.
- `apps/local-runner/internal/runner/flow_executor_test.go`: added a
  flow-engine regression proving the real synthesis prompt for
  `review-loop.yaml` includes the joined reviewer note after the cohort joins.

## Verification

- `go test ./internal/runner -run 'TestFlowEngineSynthesisPromptIncludesJoinedReviewerNote|TestCohortConsolidatedNoteEmittedOnLastMember|TestCohortFailedMemberIncludedInNote|TestCohortCompletedMemberWithoutFinalMessageGetsPlaceholder|TestE2EReviewLoopApprovedPath'` passes.
- `go test ./internal/runner -run 'TestAutoReinvokeHubSingleFlightConcurrent|TestAutoReinvokeHubNoPendingContextNoDefer|TestAutoReinvokeHubDeferredWhenCoderCompletesInFlight|TestAutoReinvokeHubTurnInFlightGuard|TestAutoReinvokeHubCapBounded|TestCreateRunDoesNotApplyEntryStepYoloToWorkflowLaunch|TestCreateRunResolvesYoloFromWorkflowWhenEnabled|TestCreateRunResolvesWorkflowYoloEvenWhenEntryStepDefinesModel|TestCreateRunResolvesYoloFromSingleStepWhenEnabled|TestStartResolvedFlowChildInheritsWorkflowYoloDefault|TestStopAgentLoopCancelsParentTurn|TestInterruptParentCancelsRunningChildAgents'` passes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-42
change_type: bugfix
summary: Make review-loop synthesis prompts carry an explicit joined reviewer note so the hub can finalize instead of hanging after both reviewers complete
# --->8---
