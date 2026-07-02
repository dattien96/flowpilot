# BUG-175: Flow Step-Timeline Sidebar Lost Whenever Main Run Is Not "running"

## Metadata

- Document ID: `BUG-175`
- Title: `Flow Step-Timeline Sidebar Lost Whenever Main Run Is Not "running"`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-168-Flow-Step-Timeline-Sidebar-Hidden-During-Approval-Or-Question.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-156-Flow-Timeline-Moved-To-Dedicated-Collapsible-Sidebar.md`, `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, flow-mode, regression, resilience`

## AI Quick View

### Summary

- In Flow Mode the step-timeline sidebar keeps vanishing at the exact moments it is most useful: while a sub-agent holds an approval (the main run goes idle, not "running"), between the coder finishing and the reviewers starting, and once the run completes.
- BUG-168 widened the visibility predicate to `running` + `waiting_approval` + `waiting_question`, but that is still status-gated — and a sub-agent's approval leaves the *main* run idle, so none of those match and the whole `<aside>` unmounts.
- Per user direction: a flow's step timeline should be shown for the entire lifetime of a started run — running, paused, errored, or completed — and hidden only before the first prompt is sent (no run yet). Status is not a visibility signal.

### Current Ask

- Keep the Flow Mode step-timeline sidebar mounted whenever a flow run exists, regardless of run status; hide it only when no run has started.

### Key Decisions

- `V-1` Visibility = `isFlowModeRun(chatMode) && (mainRunId is set || step data exists)`. A run either exists or it doesn't; `runStatus` is dropped from the predicate entirely. `resetRun` clears both `mainRunId` and `workflowStepRuntime`, so the pre-first-prompt state is the only hidden case.

### Constraints

- Pure display-logic change in one component (`FlowTimelineSidebar.tsx`); no change to run status semantics or the step-runtime data path. Does not affect `normal_chat` runs (the `isFlowModeRun` guard stays).

### Open Questions

- None. Two separate observations from the same session — a reviewer prematurely terminating the flow (join-barrier bypass) and reviewer-cohort approvals not surfacing on the main run — are tracked/investigated separately, not here (this is display-only).

### Source Refs

- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx:20-26` (`visible` predicate, pre-fix)
- `apps/desktop-flowpilot/src/state/store.ts:1542-1581` (`resetRun` clears `mainRunId` + `workflowStepRuntime`)

## 1. Issue Summary

The Flow Mode step-timeline sidebar disappears whenever the main run is not literally `running`: while a sub-agent (coder/reviewer) is paused on an approval, in the gap between the coder completing and the reviewer cohort starting, and after the run completes. The user must dismiss/advance the blocking state to get the sidebar back.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, a Flow Mode ("Review Loop") run with sub-agents and YOLO off.
- reproduction steps:
  1. Start a Flow Mode run so the step-timeline sidebar shows.
  2. Let a sub-agent request an approval (main run goes idle) — the sidebar vanishes.
  3. Approve — the sidebar reappears.
  4. Observe the same vanish between the coder finishing and reviewers starting, and after completion.
- frequency: deterministic at every non-"running" moment of a flow run.

## 4. Expected vs Actual

- expected: the step-timeline sidebar stays visible for the whole lifetime of a started flow run.
- actual: it unmounts whenever the main run is not `running`/`waiting_approval`/`waiting_question`.

## 5. Impact

- users affected: anyone using Flow Mode, especially multi-agent flows with approvals.
- workflows affected: Flow Mode UI only; no data loss, but the primary orchestration surface flickers out at the worst moments.
- severity: medium — reads as instability and removes orientation exactly when the user needs it.

## 6. Root Cause

- confirmed cause: `FlowTimelineSidebar` gated `visible` on `runStatus` (`FlowTimelineSidebar.tsx:24-26`, post-BUG-168). Approvals/questions raised by a *sub-agent* live on the child run; the main run the sidebar reads sits at `idle` (or a terminal status) at those moments, so the predicate is false and the component returns `null`. The same applies to the coder→reviewer gap and to completion.

## 7. Fix Strategy

- `F-1` Replaced the status-based predicate with an existence-based one in `FlowTimelineSidebar.tsx`: `const runStarted = Boolean(mainRunId) || steps.length > 0; const visible = isFlowModeRun(chatMode) && runStarted;`. Removed the now-unused `runStatus` selector. The sidebar now stays mounted for the entire lifetime of a flow run and hides only before the first prompt (when `resetRun` has cleared `mainRunId` and the step list).

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Not executed live: the sidebar only renders inside an active flow run (needs the runner + a provider account). The change is a one-line predicate swap backed by `resetRun`'s clearing of `mainRunId`/`workflowStepRuntime`; the user should confirm on next live use that the sidebar now stays put through approvals, the coder→reviewer gap, and completion.

## 9. Regression Guard

- tests: none (no render-test harness for this component, consistent with BUG-156/158/159/168/173).
- alerts: none.
- audit checks: recorded in `change-audit/CA-212-flow-timeline-sidebar-always-visible-once-run-started.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: supersedes BUG-168's narrower predicate (kept for history); BUG-168's `starting`-exclusion rationale no longer applies since visibility no longer keys on status.
