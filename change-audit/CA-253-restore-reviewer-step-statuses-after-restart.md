# CA-253: Restore Reviewer Step Statuses After Restart

## Scope

Fixed a live CP-36 Scenario 9 restart regression from `run-14749`: after restarting the server, the latest reviewer child sessions were still known as completed/failed, but the flow step timeline restored both reviewer nodes as `PENDING`.

## Changes

- `ProviderSessionState` / `sessions.ndjson`: added persisted child run `label` so future resume can map a child session directly back to its flow node id.
- `interactive_resume.go`: resume now prefers persisted `label`, and legacy sessions without labels are inferred from `flow_cohort_id` plus parent forward-edge order.
- `interactive_resume.go`: blocked flow runs now restore step rows from child-session evidence rather than treating the whole flow as terminal all-DONE.
- `flow_step_runtime_test.go`: added restart/disk round-trip tests for both persisted-label sessions and legacy no-label cohort sessions.

## Verification

- `rtk go test ./internal/runner -run 'TestReconstructResume(RestoresDistinctReviewerStatusesByPersistedLabel|InfersLegacyReviewerStatusesFromCohortOrder|KeepsCompletedFlowNodesAndCancelsSyntheticHub|CancelsRunningReviewerButKeepsCompletedCoder)|TestReconstruct(WorkflowRunRestoresStepTimeline|NonCompletedFlowRunRestoresPendingSteps|IncompleteCompletedFlowRunCancelsResumeAndDisablesAutoOrchestrate)' -count=1` from `apps/local-runner` — passed.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-256
change_type: bugfix
summary: persist child flow node labels and infer legacy cohort labels so reviewer step statuses restore after restart
# --->8---
