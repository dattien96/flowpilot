# BUG-178: Flow Step Timeline Empty After Server Restart (History Run)

## Metadata

- Document ID: `BUG-178`
- Title: `Flow Step Timeline Empty After Server Restart (History Run)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-170-Workflow-Flow-Mode-History-Unresumable-After-Server-Restart.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`, `requirements/09-BugFix/done/BUG-175-Flow-Step-Timeline-Sidebar-Lost-Whenever-Main-Run-Not-Running.md`
- Replaces: `none`
- Tags: `agent-flow-engine, runner, flow-mode, resume, history, step-runtime`

## AI Quick View

### Summary

- After the runner restarts, reopening a completed flow-mode run from history shows the step-timeline sidebar with "0/0 steps — No step-runtime data for this run yet." The run resumes (BUG-170) but its step timeline is empty.
- Root cause: the local runner keeps step-runtime state in an **in-memory** `fakeWorkflowStore`, so a run's step list is gone after a restart. `reconstructRun` re-seeds a step only for `chat` runs (BUG-170's F-1b); for workflow/flow runs its comment assumed the store "keeps whatever state it already has" — true for a hypothetical persistent store, false for the in-memory one.
- Fix: on resume, rebuild the step list from the flow nodes already persisted on the session (`ActiveFlowNodes`, restored onto the run). A completed run's steps are restored DONE; a non-completed run's PENDING (per-step progress isn't persisted).

### Current Ask

- A flow-mode history run must show its step timeline after a server restart, not "No step-runtime data".

### Key Decisions

- `V-1` `reconstructRun` rebuilds the step-runtime list from `rs.activeFlowNodes` for any non-chat run that has them; status = DONE when the resumed run is `completed`, else PENDING. Gated on `len(activeFlowNodes) > 0`, so plain (non-flow) workflow runs are unaffected (they keep BUG-170's no-seed behavior).

### Constraints

- Restores the timeline display only; it does not re-engage the flow executor on resume (`flowEngineDriven` left as-is). Per-step statuses for a mid-flow resume are approximate (all PENDING) because per-step progress is not persisted — only the completed case (the reported one) is exact.
- No persistence schema change: reuses `ActiveFlowNodes`, already persisted for edge-driven routing after restart (BUG-NOTE-CP42 #16).

### Open Questions

- Exact per-step status fidelity for a non-completed resumed flow would require persisting the step snapshot (a schema addition + write churn). Deferred — the reported case is a completed run, which is restored exactly.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go:389` (`ActiveFlowNodes` restored onto the resumed run)
- `apps/local-runner/internal/runner/interactive_resume.go:426-435` (pre-fix: re-seed only for `chat` runs)
- `apps/local-runner/internal/runner/interactive_service.go:328-329` (`workflowStore` falls back to the in-memory `fakeWorkflowStore`)
- `apps/local-runner/internal/runner/interactive_handlers.go:1018` (`handleGetWorkflowStepsRuntime` → `LoadRunSteps`, empty after restart)

## 1. Issue Summary

Restart the runner, reopen a completed flow-mode ("Review Loop") run from history: the step-timeline sidebar shows "No step-runtime data for this run yet" instead of the run's completed steps.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local runner; a completed flow-mode run recorded in history.
- reproduction steps:
  1. Run a flow-mode ("Review Loop") session to completion.
  2. Restart the runner (clears the in-memory step store).
  3. Reopen that run from history.
  4. Observe the step-timeline sidebar reads "0/0 steps — No step-runtime data for this run yet."
- frequency: deterministic for flow runs after a restart.

## 4. Expected vs Actual

- expected: the step timeline shows the run's steps (all DONE for a completed run).
- actual: empty timeline, "No step-runtime data for this run yet."

## 5. Impact

- users affected: anyone reopening a flow-mode run after a runner restart.
- workflows affected: flow-mode history review across restarts; the run resumes (BUG-170) but its orchestration timeline is lost.
- severity: medium — data-presence gap; the history entry loads but the timeline is blank.

## 6. Root Cause

- confirmed cause: the local runner's `workflowStore` is the in-memory `fakeWorkflowStore` (`interactive_service.go:328-329`), so per-run step state does not survive a restart. `reconstructRun` re-seeds a step list only for `chat` runs (`interactive_resume.go:426-435`); its comment ("workflow runs keep whatever step-runtime state their store already has") assumed a persistent store. With the in-memory store empty after restart, `LoadRunSteps` returns nothing and `handleGetWorkflowStepsRuntime` yields zero steps.

## 7. Fix Strategy

- `F-1` Added `reseedFlowStepRuntimeForResume` (`flow_step_runtime.go`): rebuilds one step-runtime row per persisted flow node (`ID == NodeID`), DONE when the run is completed (stamped started/finished), else PENDING. Factored the row builder (`flowStepRowsFromNodes`) shared with the executor's live reseed.
- `F-2` `reconstructRun` (`interactive_resume.go`) now calls it for a non-chat run with `rs.activeFlowNodes` populated, passing `completed = rs.status == RunStatusCompleted`. Gated on having nodes, so plain workflow runs keep BUG-170's behavior.

## 8. Validation

- `V-1` **(done)** `TestReconstructWorkflowRunRestoresStepTimeline`: a persisted completed workflow run with `ActiveFlowNodes` reconstructs with all four steps present and DONE. `TestReconstructNonCompletedFlowRunRestoresPendingSteps`: a non-completed run restores the steps as PENDING. Both pass.
- `V-2` **(done)** No regression: BUG-170's `TestResumeRunReconstructs*` still pass (a workflow run with no `ActiveFlowNodes` still seeds nothing); the broad flow/step/resume subset → 337 passed. `go build` clean.
- `V-3` **(not executed)** A live runner restart + reopen through the desktop, confirming the sidebar now shows the completed steps — no runner + provider account in this environment.

## 9. Regression Guard

- tests: `TestReconstructWorkflowRunRestoresStepTimeline`, `TestReconstructNonCompletedFlowRunRestoresPendingSteps`; BUG-170's reconstruct tests guard the no-nodes path.
- alerts: none.
- audit checks: recorded in `change-audit/CA-215-restore-flow-step-timeline-on-resume.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: BUG-170's chat-only re-seed comment is superseded for flow runs by this fix; exact per-step status for a non-completed resume is left as a deferred persistence enhancement (see Open Questions).
