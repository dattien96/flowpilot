# BUG-256: Restarted Flow Step Timeline Loses Reviewer Statuses When Nodes Share Agent

## Metadata

- Document ID: `BUG-256`
- Title: `Restarted Flow Step Timeline Loses Reviewer Statuses When Nodes Share Agent`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 9)
- Child Documents: `none`
- Related Documents: [BUG-254: Failed Cohort Reviewer Marked Done And Hidden Node Identity](./BUG-254-Failed-Cohort-Reviewer-Mark-Done-And-Hidden-Node-Identity.md), [BUG-255: Coder Reentry Output Duplicated Into Main After Stale Loop State](./BUG-255-Coder-Reentry-Output-Duplicated-Into-Main-After-Stale-Loop-State.md)
- Replaces: `none`
- Tags: `agent-flow-engine, resume, step-runtime, cohort, scenario-9`

## AI Quick View

### Summary

- Found during live CP-36 Scenario 9 verification on `run-14749`: after restarting the server, the Agents panel still showed the latest reviewer children as `completed` and `failed`, but the flow step timeline showed both reviewer nodes as `PENDING`.
- The live run had correctly transitioned `review-gpt` to `DONE` and `review-security-gpt` to `FAILED` before restart.
- Resume rebuilt the in-memory step timeline from `sessions.ndjson`, but old child session records did not persist the flow node label. Both reviewer children had the same `agent_name=reviewer-agent`, so resume could not map them back to distinct nodes.
- Additionally, a `blocked` loop was treated as terminal for step restore and could incorrectly restore every node as `DONE` instead of using child-session evidence.

### Current Ask

- On restart, restore reviewer node statuses from disk so the latest reviewer round shows the same truth as the Agents panel: one `DONE`, one `FAILED`, not two `PENDING`.

### Key Decisions

- `V-1` Persist child run `label` into `ProviderSessionState` and `sessions.ndjson` so future restarts can map child sessions directly to flow node ids.
- `V-2` For legacy session records without `label`, infer cohort node identity from `flow_cohort_id` plus the parent flow's forward edges and spawn order.
- `V-3` Treat `blocked` flow runs as paused/incomplete for step restore: use child-session evidence instead of restoring every node as `DONE`.

### Constraints

- Keep the fix scoped to resume display reconstruction; do not resume auto-orchestration after restart.
- Preserve completed flow behavior: truly `done` flows still restore all nodes as `DONE`.
- Preserve future runs by writing explicit `label`; legacy inference exists only for already persisted sessions.

### Open Questions

- none

### Source Refs

- Live feature log: `.flowpilot/logs/features/agent-flow-engine/run-14749.ndjson`
- Session manifest: `.flowpilot/chats/sessions.ndjson`
- [interactive_resume.go](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go)
- [workflow_store.go](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/workflow_store.go)
- [local_file_session_store.go](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/local_file_session_store.go)
- [flow_step_runtime_test.go](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_step_runtime_test.go)

## 1. Issue Summary

After `run-14749` reached the cap and the server was restarted, the right-side Agents panel had enough child-session status evidence to show reviewer children as `completed` and `failed`. The flow step timeline did not: both reviewer nodes were displayed as `PENDING`.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 9
- impacted task: none directly
- impacted tech design: none directly
- impacted system spec: none directly

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local-runner
- flow: custom two-reviewer workflow `flow-gpt-2-review`
- run: `run-14749`
- reproduction steps:
  1. Run a flow where `coder-gpt` fans out to `review-gpt` and `review-security-gpt`.
  2. Let `review-gpt` complete and `review-security-gpt` fail.
  3. Continue until the flow reaches the cap/blocked state.
  4. Stop and restart the server.
  5. Reopen the run.

## 4. Expected vs Actual

- expected: `review-gpt` restores as `DONE`; `review-security-gpt` restores as `FAILED`.
- actual: both reviewer rows restored as `PENDING`.

## 5. Impact

- users affected: users reopening flow-engine runs after runner restart
- workflows affected: custom reviewer cohorts where multiple nodes share the same agent definition
- severity: medium — execution had already happened correctly, but the restored timeline lied about reviewer outcomes

## 6. Root Cause

- confirmed cause 1: `ProviderSessionState` did not persist `interactiveRun.label`, so child sessions in `sessions.ndjson` could not identify their flow node after restart.
- confirmed cause 2: `matchFlowNodeForSession` fell back to matching by `AgentName`/`Role`; when two flow nodes shared `reviewer-agent`, it returned no match because the mapping was ambiguous.
- confirmed cause 3: `resumedFlowStepRows` treated `LoopState.Status == "blocked"` as terminal and could restore all nodes `DONE`, even though a blocked cap state is an incomplete flow waiting on user action.

## 7. Fix Strategy

- `F-1` Add `Label` to `ProviderSessionState` and the local NDJSON session record.
- `F-2` Populate `Label` from `interactiveRun.label` in `sessionStateOf`.
- `F-3` Restore `interactiveRun.label` from persisted `ProviderSessionState.Label`.
- `F-4` Make resume node matching prefer `session.Label`.
- `F-5` Add legacy fallback: for old sessions without labels, derive target node ids from `flow_cohort_id` (`flow-auto-<source>-round-<n>`) and parent forward edges, then map cohort child sessions by spawn order.
- `F-6` For `blocked` loops, rebuild step rows from child-session evidence instead of restoring every node as `DONE`.

## 8. Validation

- `V-1` `rtk go test ./internal/runner -run 'TestReconstructResume(RestoresDistinctReviewerStatusesByPersistedLabel|InfersLegacyReviewerStatusesFromCohortOrder|KeepsCompletedFlowNodesAndCancelsSyntheticHub|CancelsRunningReviewerButKeepsCompletedCoder)|TestReconstruct(WorkflowRunRestoresStepTimeline|NonCompletedFlowRunRestoresPendingSteps|IncompleteCompletedFlowRunCancelsResumeAndDisablesAutoOrchestrate)' -count=1` from `apps/local-runner` — `7` tests passed.

## 9. Regression Guard

- tests:
  - `TestReconstructResumeRestoresDistinctReviewerStatusesByPersistedLabel`
  - `TestReconstructResumeInfersLegacyReviewerStatusesFromCohortOrder`
- impact analysis:
  - `rtk npx gitnexus impact --repo flowpilot ProviderSessionState` — `CRITICAL`; change is additive field only.
  - `rtk npx gitnexus impact --repo flowpilot sessionStateOf` — `CRITICAL`; change is additive label snapshot only.
  - `rtk npx gitnexus impact --repo flowpilot reconstructRun` — `LOW`
  - `rtk npx gitnexus impact --repo flowpilot matchFlowNodeForSession` — `LOW`
  - `rtk npx gitnexus impact --repo flowpilot resumedFlowStepRows` — `LOW`

## 10. Follow-Up Document Updates

- upstream docs updated: CP-36 Scenario 9 now references this restart/resume fix.
