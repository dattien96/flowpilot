# BUG-254: Failed Cohort Reviewer Marked Done And Hidden Node Identity

## Metadata

- Document ID: `BUG-254`
- Title: `Failed Cohort Reviewer Marked Done And Hidden Node Identity`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 9), [Task-189: Custom Flow Graph Authoring](../../08-Task/done/Task-189-Custom-Flow-Graph-Authoring.md)
- Child Documents: `none`
- Related Documents: [BUG-253: Cloned Built-In Workflow Edit/Save Hits Workflow Step Order Unique Constraint](./BUG-253-Cloned-Builtin-Workflow-Edit-Save-Hits-Workflow-Step-Order-Unique-Constraint.md), [CA-236: Review Loop Runaway Advance And Cohort Node Settlement](../../../change-audit/CA-236-review-loop-runaway-advance-and-cohort-node-settlement.md)
- Replaces: `none`
- Tags: `agent-flow-engine, cohort, flow-timeline, agents-panel, regression, scenario-9`

## AI Quick View

### Summary

- Found during live CP-36 Scenario 9 verification on custom flow `flow-gpt-2-review`, run `run-13821`: one reviewer (`review-security-gpt`) intentionally failed due unsupported Codex model `gpt-5.2`, while the other reviewer completed and the flow correctly continued.
- Backend first marked the failed node `FAILED`, but when the sibling reviewer completed and the cohort joined, the completed-path join code bulk-marked every cohort entry `DONE`, including the failed one.
- Later, after `continue` started a new round and the flow completed without rerunning that reviewer, `markFlowRunComplete` bulk-marked every non-terminal step `DONE`, so a `PENDING` reviewer step from the skipped round also became falsely green.
- Agents panel showed both reviewer children as generic `reviewer-agent`, so the failed `review-security-gpt` node identity was not visible in Recently Closed.

### Current Ask

- Preserve failed reviewer status through cohort join and flow completion, and show the flow node label in the Agents panel so two reviewer nodes using the same agent definition remain distinguishable.

### Key Decisions

- `V-1` Cohort join may mark only `completed` entries `DONE`; failed entries are already terminal and must not be overwritten.
- `V-2` Flow completion must keep existing terminal statuses, mark active work `DONE`, and mark unstarted `PENDING` work `SKIPPED` rather than pretending it completed.
- `V-3` `AgentRunSummary` carries an optional `label` field, populated from the flow node id (`interactiveRun.label`), and the desktop Agents panel prefers that label over the generic agent name.

### Constraints

- Do not regress BUG-181/BUG-233 ordering guarantees: final step settlement still happens before the terminal `agent_graph_updated` event.
- Do not change cohort barrier semantics: one failed member plus one completed member still joins and lets synthesis proceed.
- Do not rename or split agent definitions just to make UI labels unique; node identity already exists as the flow label.

### Open Questions

- The main transcript in run `run-13821` displayed substantial coder output after coder re-entry. The log does not prove the active run switched to the coder; it may be parent timeline annotation/context rendering. This remains worth a separate UI transcript-routing pass if it reproduces after the status fixes.

### Source Refs

- Live feature log: `.flowpilot/logs/features/agent-flow-engine/run-13821.ndjson`
- [interactive_service.go](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go)
- [flow_step_runtime.go](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_step_runtime.go)
- [agent_orchestrator.go](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/agent_orchestrator.go)
- [AgentsPanel.tsx](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/desktop-flowpilot/src/components/AgentsPanel.tsx)

## 1. Issue Summary

During live Scenario 9 verification, the owner created a custom flow with two reviewer delegate nodes and intentionally configured one reviewer to fail. The core Scenario 9 behavior was verified: the remaining reviewer completed, the cohort joined, and synthesis proceeded despite one reviewer failure. However, the UI timeline and Agents panel exposed follow-up bugs:

- `review-security-gpt` was shown as failed initially, then became done after the sibling reviewer completed.
- At final flow completion, the failed/unrun reviewer still appeared done in the timeline.
- Recently Closed showed generic `reviewer-agent` rows, so the specific failed node (`review-security-gpt`) was not identifiable.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 9
- impacted task: [Task-189](../../08-Task/done/Task-189-Custom-Flow-Graph-Authoring.md), custom two-reviewer flow authoring
- impacted tech design: none directly
- impacted system spec: none directly

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local-runner, real project target `/Users/tiendat/Desktop/BE/gate-sandbox`, custom workflow `flow-gpt-2-review`
- reproduction steps:
  1. Create/run a flow with `coder-gpt` fanning out to `review-gpt` and `review-security-gpt`, then `synthesis`.
  2. Configure one reviewer to fail; in `run-13821`, `review-security-gpt` used Codex model `gpt-5.2`, producing a provider error.
  3. Let the other reviewer complete.
  4. Observe timeline and Agents panel through synthesis/continue/final done.
- frequency: deterministic for the logged sequence: failed cohort member first, completed sibling second, then final flow completion with unstarted downstream nodes.

## 4. Expected vs Actual

- expected: failed reviewer remains `FAILED`; a reviewer node that never runs in a later round is not shown as `DONE`; Agents panel distinguishes `review-gpt` from `review-security-gpt`.
- actual: cohort join and final completion overwrite the failed/unrun node to `DONE`; Agents panel shows both children as `reviewer-agent`.

## 5. Impact

- users affected: users verifying or operating custom multi-reviewer flows
- workflows affected: CP-36 review cohort flows, custom fan-out reviewer workflows, Agents panel inspection
- severity: medium — execution can continue correctly, but observability lies about which reviewer failed or actually ran

## 6. Root Cause

- confirmed cause 1: in the completed-member cohort-join path, the code built `reviewerNodeIDs` from every drained cohort entry with a label, regardless of `cohortEntry.Status`. It then set every collected node id to `StepStatusDone`, overwriting a previously failed reviewer.
- confirmed cause 2: `markFlowRunComplete` set every non-terminal step to `DONE`. After `continue`, downstream nodes are reset to `PENDING` for the next round; if the flow is later completed without running those nodes, `PENDING` was falsely converted to `DONE`.
- confirmed cause 3: `AgentRunSummary` did not expose the flow node label, so desktop could only render `agentName` (`reviewer-agent`) even when two separate flow nodes used the same agent definition.
- evidence from `run-13821`:
  - `04:44:41`: `review-security-gpt` receives `step_status_transition` to `FAILED`
  - `04:46:57`: after `cohort_join_complete`, `review-security-gpt` receives `step_status_transition` to `DONE`
  - `04:49:39`: final `flow_run_complete_begin` bulk-marks `review-security-gpt` `DONE` again

## 7. Fix Strategy

- `F-1` Filter cohort-join step settlement to `cohortEntry.Status == "completed"` before setting a reviewer node `DONE`.
- `F-2` Update `markFlowRunComplete` so `DONE`, `FAILED`, `SKIPPED`, and `CANCELED` remain unchanged; `PENDING` becomes `SKIPPED`; other active statuses become `DONE`.
- `F-3` Treat `SKIPPED` as terminal in `setFlowStepStatus` so it gets `finished_at`.
- `F-4` Add optional `label` to backend/frontend `AgentRunSummary`, populate it from `interactiveRun.label`, and have `AgentsPanel` display `label || agentName`.

## 8. Validation

- `V-1` `rtk go test ./internal/runner -run 'TestCohortFailedMemberIncludedInNote|TestMarkFlowRunComplete(SettlesEveryStep|PreservesFailedAndSkipsUnstartedSteps)' -count=1` from `apps/local-runner` — `3` tests passed.
- `V-2` `rtk go test ./internal/runner -run 'TestCohortMemberSettlesOwnNodeOnCompletion|TestApplyFlowControlDoneTerminates' -count=1` from `apps/local-runner` — `2` tests passed.
- `V-3` `rtk npm --prefix apps/desktop-flowpilot run typecheck` — passed.
- `V-4` `rtk npx tsx --test apps/desktop-flowpilot/src/components/AgentsPanel.test.ts` was attempted but cannot run raw in this repo because the test imports `AgentsPanel.tsx`, which uses the Vite alias `@/state/store`; raw `tsx` did not resolve that alias. Desktop typecheck covers the new field/helper.

## 9. Regression Guard

- tests:
  - `TestCohortFailedMemberIncludedInNote` now also asserts the failed cohort member remains `FAILED` after the completed sibling triggers cohort join
  - `TestMarkFlowRunCompletePreservesFailedAndSkipsUnstartedSteps`
  - `agentRunDisplayName prefers flow node label over generic agent name`
- alerts: none
- audit checks: GitNexus MCP tools were unavailable in this thread; blast radius was assessed by direct caller search around `markFlowRunComplete`, cohort join, `AgentRunSummary`, and `AgentsPanel`

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-36 Scenario 9 can record `run-13821` as live evidence that one failed reviewer does not block synthesis, with BUG-254 as the follow-up observability fix.
- notes left unchanged on purpose: this bug does not change the flow execution rule that a failed cohort member counts as settled for the barrier.
