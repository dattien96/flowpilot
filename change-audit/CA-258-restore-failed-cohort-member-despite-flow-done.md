# CA-258: Restore Failed Cohort Member Despite Flow Done

## Scope

Fixed a live CP-36 Scenario 9 restart regression from `run-10239` (Claude): a cohort reviewer that genuinely failed restored as `DONE` after a server restart, because the flow's own loop state had reached a genuinely terminal `"done"` (the hub can legitimately proceed off surviving cohort members, per CA-251/BUG-254).

## Changes

- `interactive_resume.go`: `resumedFlowStepRows` no longer short-circuits to an unconditional all-DONE fast path when the flow's loop state is terminal and non-blocked. It now always runs the per-child session-evidence walk first (DONE/FAILED/CANCELED per node, unchanged logic), and only applies "flow completed" as a fallback default — for the inline hub node when it has no child session, and for any other node still `PENDING` after the evidence walk (no session matched at all). Real evidence, including `FAILED`, always wins over that fallback.
- `flow_step_runtime_test.go`: added `TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone`, reproducing the exact live shape (one completed + one failed reviewer, parent loop state `"done"`) and asserting the failed reviewer restores as `FAILED`.

## Verification

- `rtk go test ./internal/runner -run 'TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone' -v -count=1` from `apps/local-runner` — passed; confirmed to fail against the pre-fix fast path via a temporary scoped revert.
- `rtk go test ./internal/runner -run 'TestReconstruct' -v -count=1` from `apps/local-runner` — all 10 `TestReconstruct*` tests passed, confirming no regression to BUG-256/BUG-257's own restore scenarios.
- `rtk go test ./internal/runner/... -count=1` from `apps/local-runner` — 1144 passed, same 15 pre-existing unrelated failures; no new failures.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-260
change_type: bugfix
summary: unify resumed flow step restore so a genuinely completed flow's fast-path DONE default only fills gaps with no session evidence, never overwriting a cohort member's real FAILED outcome
# --->8---
