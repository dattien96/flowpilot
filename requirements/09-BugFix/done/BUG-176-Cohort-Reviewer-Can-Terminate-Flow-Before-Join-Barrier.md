# BUG-176: Cohort Reviewer Can Terminate The Flow Before The Join Barrier

## Metadata

- Document ID: `BUG-176`
- Title: `Cohort Reviewer Can Terminate The Flow Before The Join Barrier`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`, `requirements/09-BugFix/done/BUG-175-Flow-Step-Timeline-Sidebar-Lost-Whenever-Main-Run-Not-Running.md`
- Replaces: `none`
- Tags: `agent-flow-engine, runner, flow-mode, review-loop, orchestration, join-barrier`

## AI Quick View

### Summary

- In a review-loop, the step timeline showed the "synthesis" (hub) node DONE and the run terminated while both reviewer nodes were still RUNNING — a logically impossible ordering (the hub must wait for the whole review cohort, then synthesize).
- Root cause: `turnBridge.SubmitFlowControl` routed a child's flow-control call straight to the parent's `applyFlowControl` with **no cohort-join gate**. A single reviewer calling `submit_review_outcome` mid-turn therefore advanced/terminated the whole round before its sibling finished — bypassing the join barrier `maybeAutoReinvokeHub` depends on. Made visible by BUG-174's honest step timeline (`markFlowRunComplete` marked synthesis DONE on that premature `done`).
- Only the hub's own synthesis turn — which runs after the cohort joins and is not a cohort member — may finalize a flow. This enforces that in code, backstopping the existing hub-only tool-advertisement gate (BUG-NOTE-CP42 #24) and the auto-spawn prompt that already tells reviewers not to call the tool (BUG-NOTE-CP42 #13).

### Current Ask

- A cohort member (reviewer) must not be able to drive the flow's control tool; only the hub, after the join, may advance/terminate the flow.

### Key Decisions

- `V-1` `SubmitFlowControl` rejects a call from a cohort-member run (`flowCohortId != ""`) with instructive guidance, so its findings flow through its final message into the cohort note the hub synthesizes. Non-cohort children and the hub (the parent run, `flowCohortId == ""`) are unaffected.

### Constraints

- Runner-only, behavior-preserving for every non-cohort caller: the hub's own synthesis turn still finalizes the flow; a non-cohort child that legitimately drives flow-control still routes to the parent unchanged.
- Prompt- and advertisement-level guards already existed but could not *enforce* the barrier (a model can call an unadvertised tool); this closes it at the execution layer.

### Open Questions

- Whether to additionally fold a rejected reviewer's structured outcome payload into the cohort note (today its findings reach the hub via its final message, which the auto-spawn prompt already asks for). Deferred — not needed for correctness.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go:1776` (`turnBridge.SubmitFlowControl` — pre-fix: child → parent `applyFlowControl`, no gate)
- `apps/local-runner/internal/runner/interactive_service.go:1400-1406` (cohort join → `maybeAutoReinvokeHub`, the barrier being bypassed)
- `apps/local-runner/internal/runner/flow_executor.go` (BUG-NOTE-CP42 #13: the auto-spawn reviewer prompt already avoids instructing a control-tool call)
- `apps/local-runner/internal/runner/claude_mcp_server.go` / `codex_appserver` tests (BUG-NOTE-CP42 #24: `submit_review_outcome` advertised for hub turns only)

## 1. Issue Summary

During a review-loop run, the hub's "synthesis" step and the whole run were marked complete while the reviewer cohort was still running, because a reviewer could call the flow's control tool and terminate the round on its own instead of the hub deciding after every reviewer finished.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: local runner, a Flow-Mode review-loop with a two-reviewer cohort, YOLO off.
- reproduction steps:
  1. Run a review-loop so the coder completes and both reviewers spawn as a cohort.
  2. Have one reviewer call `submit_review_outcome` (flow control) before the other finishes.
  3. Observe the flow terminate / synthesis marked DONE while the sibling reviewer is still RUNNING.
- frequency: whenever a cohort member emits a flow-control call before the join.

## 4. Expected vs Actual

- expected: the hub waits for the entire review cohort to finish, synthesizes, and only then calls the control tool to continue/finish/escalate.
- actual: any single reviewer's control-tool call immediately advanced/terminated the flow.

## 5. Impact

- users affected: anyone running a multi-reviewer flow (Review Loop and similar cohorts).
- workflows affected: the review loop's core correctness — findings from a still-running reviewer are dropped when a sibling short-circuits the round.
- severity: high — silently skips part of the review and misreports the flow as complete.

## 6. Root Cause

- confirmed cause: `turnBridge.SubmitFlowControl` (`interactive_service.go:1776`) resolved the target run (child → parent) and called `applyFlowControl` unconditionally. Cohort membership (`flowCohortId`) was never consulted, so a reviewer's call reached the hub's loop state directly, ahead of the `cohortComplete` → `drainCohort` → `maybeAutoReinvokeHub` join (`interactive_service.go:1400-1406`). The tool was only *advertised* to hub turns (BUG-NOTE-CP42 #24) and the auto-spawn prompt already asked reviewers not to call it (BUG-NOTE-CP42 #13), but neither can prevent a model from invoking it.

## 7. Fix Strategy

- `F-1` `SubmitFlowControl` now returns an error for a caller whose run is a cohort member (`b.rs.flowCohortId != ""`), with a message instructing it to report findings in its final message and let the hub finalize after the join. The hub's own turn (parent run, `flowCohortId == ""`) and non-cohort children are unchanged (`interactive_service.go:1776`).

## 8. Validation

- `V-1` **(done)** `TestSubmitFlowControlRejectsCohortMemberButAllowsHub` (`flow_step_runtime_test.go`): a cohort member's `flow_control(done)` is rejected and does not move the loop to `done`; the hub's own `flow_control(done)` still finalizes it. Passes.
- `V-2` **(done)** No regression: `go test ./internal/runner/ -run 'Flow|Workflow|Orchestrat|Cohort|Coder|Reviewer|Progress|StepRuntime|Advance|Resolve|SubmitFlowControl|Hub|Review|Loop'` → 314 passed, including the existing cohort/join, continue-reinvoke, and hub-advertisement tests.
- `V-3` **(not executed)** A live review-loop confirming the hub now always waits for both reviewers before synthesis — no runner + provider account in this environment.

## 9. Regression Guard

- tests: `TestSubmitFlowControlRejectsCohortMemberButAllowsHub`; existing cohort-join / `maybeAutoReinvokeHub` / hub-only advertisement tests guard the surrounding behavior.
- alerts: none.
- audit checks: recorded in `change-audit/CA-213-cohort-join-barrier-enforced-in-flow-control.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this enforces CP-36's intended hub-drives-all-transitions design rather than changing it.
- notes left unchanged on purpose: reviewer findings still reach the hub via the reviewer's final message + cohort note (the existing path); folding the rejected tool payload into the note is a deferred nicety (see Open Questions).
