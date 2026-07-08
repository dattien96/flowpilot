# CA-251: Preserve Failed Cohort Reviewer Status

## Scope

Fixed a live CP-36 Scenario 9 regression found in `run-13821`: a failed reviewer in a custom two-reviewer cohort was initially marked `FAILED`, but the sibling reviewer's completion and the final flow completion later made the failed/unrun node appear `DONE`. The Agents panel also showed both reviewer children only as `reviewer-agent`, hiding which flow node failed.

## Changes

- `interactive_service.go`: cohort join now only marks entries with `Status == "completed"` as `DONE`, leaving failed entries untouched.
- `flow_step_runtime.go`: `markFlowRunComplete` now preserves terminal statuses, marks `PENDING` nodes `SKIPPED`, and only turns active/in-flight nodes into `DONE`; `StepStatusSkipped` now gets a terminal `finished_at`.
- `AgentRunSummary`: added optional `label`, populated from `interactiveRun.label` in live summary paths.
- `AgentsPanel.tsx`: displays `label || agentName`, so custom flow nodes like `review-security-gpt` are distinguishable even when they share the same `reviewer-agent` definition.

## Verification

- `rtk go test ./internal/runner -run 'TestCohortFailedMemberIncludedInNote|TestMarkFlowRunComplete(SettlesEveryStep|PreservesFailedAndSkipsUnstartedSteps)' -count=1` from `apps/local-runner` — passed.
- `rtk go test ./internal/runner -run 'TestCohortMemberSettlesOwnNodeOnCompletion|TestApplyFlowControlDoneTerminates' -count=1` from `apps/local-runner` — passed.
- `rtk npm --prefix apps/desktop-flowpilot run typecheck` — passed.
- Raw `tsx` execution of `AgentsPanel.test.ts` is blocked by unresolved Vite `@/` alias in this repo's direct test invocation; typecheck covers the changed frontend contract.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-254
change_type: bugfix
summary: preserve failed cohort reviewer step status through join and completion, and show flow node labels in the Agents panel
# --->8---
