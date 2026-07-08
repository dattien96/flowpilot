# CA-254: Carry Loop State Through Post-Turn Persist

## Scope

Fixed a live CP-36 Scenario 5 restart regression from `run-7804`: after a Review Loop ran to full completion and the server was restarted, the `synthesis` (hub) step restored as `cancelled` instead of `done`, even though the flow had genuinely finished.

## Changes

- `interactive_service.go`: `runTurn`'s post-turn persist now patches `snap.LoopState` from `agentOrchestrator.graphSnapshot` for parent/hub runs before calling `persistProviderSession`, mirroring the existing `startTurn`/`snapshotWithLoop`/`persistParentSession` pattern. Previously this trailing write used `sessionStateOf(rs)` unpatched, silently zeroing an already-terminal `"done"` loop state written moments earlier by `applyFlowControl`, which made `resumedFlowRunIncomplete` misread a fully-completed run as still in-flight on the next restart.
- `interactive_service_e2e_test.go`: added `TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes`, which drives a full Review Loop to completion against a real `LocalFileSessionStore` and asserts the latest persisted record carries `LoopState.Status == "done"` — exactly what a restart's resume path reads.

## Verification

- `rtk go test ./internal/runner -run 'TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes' -count=1` from `apps/local-runner` — passed; confirmed to fail against the pre-fix code via a temporary revert.
- `rtk go test ./internal/runner -run 'TestReconstruct|TestE2EReviewLoop|TestApplyFlowControl|TestMarkFlowRunComplete|TestCohort|TestSubmitFlowControl' -count=1` from `apps/local-runner` — 45 passed, confirming no regression to the Scenario 1/2 restart cases or the CA-251/252/253 fixes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-257
change_type: bugfix
summary: carry live loop state through the post-turn persist so a fully-completed flow's hub step restores DONE, not CANCELED, after a restart
# --->8---
