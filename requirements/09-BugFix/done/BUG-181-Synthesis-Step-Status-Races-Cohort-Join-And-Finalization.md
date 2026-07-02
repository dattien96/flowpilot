# BUG-181: Synthesis Step Status Races Cohort-Join Writes And Finalization

## Metadata

- Document ID: `BUG-181`
- Title: `Synthesis Step Status Races Cohort-Join Writes And Finalization`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-179-Hub-Finalizes-Flow-On-First-Turn-Before-Review-Cohort.md`, `requirements/09-BugFix/done/BUG-176-Cohort-Reviewer-Can-Terminate-Flow-Before-Join-Barrier.md`
- Replaces: `none`
- Tags: `agent-flow-engine, runner, flow-mode, review-loop, race, step-runtime`

## AI Quick View

### Summary

- Two opposite symptoms of one race: sometimes the synthesis (hub) step showed DONE while the reviewer cohort was still RUNNING; other times the flow was done but synthesis was left stuck RUNNING.
- Root cause: on cohort join, the step writes ("reviewers DONE, synthesis RUNNING") were dispatched in one goroutine while the hub reinvoke (`maybeAutoReinvokeHub` → synthesis turn → `markFlowRunComplete` → synthesis DONE) was dispatched in a **separate** goroutine. With no ordering, `markFlowRunComplete` could land before the reviewer-DONE writes (synthesis DONE while reviewers RUNNING) or after the synthesis-RUNNING write (synthesis stuck RUNNING after done).
- Fix: sequence the cohort step writes **before** the hub reinvoke in one goroutine, and make `markFlowRunComplete` authoritative — settle every non-terminal step to DONE on completion.

### Current Ask

- The step timeline must be consistent through cohort join and finalization: reviewers settle DONE before synthesis transitions, and on completion every step is DONE.

### Key Decisions

- `V-1` The cohort-join handler runs the step writes (reviewers DONE, synthesis RUNNING) and then `maybeAutoReinvokeHub` in a single ordered goroutine, so the synthesis turn's finalization can never precede them.
- `V-2` `markFlowRunComplete` loads the run's steps and marks every non-terminal one DONE (not just the hub node), so any lagging write can't leave the timeline inconsistent after the run is done.

### Constraints

- Runner-only; scoped to flowEngineDriven runs. No change to the orchestration/loop semantics — only the ordering of the step-runtime writes and the terminal settle.

### Open Questions

- None. The ordering removes the race; the authoritative terminal settle is belt-and-suspenders for any residual lag.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go:1404-1427` (cohort-join: step writes + reinvoke — previously two unordered goroutines)
- `apps/local-runner/internal/runner/flow_step_runtime.go` (`markFlowRunComplete` — now settles all non-terminal steps)

## 1. Issue Summary

During a review-loop, the synthesis step's status did not reliably track reality: it could appear DONE while both reviewers were still RUNNING, or remain RUNNING after the flow had already completed.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: local runner, a Flow-Mode review-loop, YOLO off.
- reproduction steps:
  1. Run a review-loop through the reviewer cohort to synthesis.
  2. Intermittently observe either (a) synthesis DONE while reviewers RUNNING, or (b) synthesis stuck RUNNING after the flow reported complete.
- frequency: intermittent (timing-dependent race).

## 4. Expected vs Actual

- expected: reviewers settle DONE, then synthesis runs, then synthesis + run settle DONE together on completion.
- actual: synthesis status could lead or lag the true state due to the race.

## 5. Impact

- users affected: anyone running a cohort flow (review-loop).
- workflows affected: Flow Mode step timeline accuracy near cohort join / completion.
- severity: medium — misleading timeline, though the flow itself completes.

## 6. Root Cause

- confirmed cause: in the `EventMessageCompleted` cohort-join branch (`interactive_service.go:1404-1427`), the step-status writes were dispatched via one `go func(){…}` and the hub reinvoke via a separate `go s.maybeAutoReinvokeHub(...)`. The reinvoke leads to `markFlowRunComplete` (synthesis DONE) at the end of the synthesis turn. With two independent goroutines there was no happens-before between the reviewer-DONE/synthesis-RUNNING writes and the synthesis-DONE finalize — either order could win.

## 7. Fix Strategy

- `F-1` Merge the cohort-join step writes and the reinvoke into one goroutine, writes first: `go func(){ …reviewers DONE; synthesis RUNNING…; s.maybeAutoReinvokeHub(parentRunID) }()` (`interactive_service.go`). The writes are committed before the synthesis turn is even scheduled, so `markFlowRunComplete` always lands last.
- `F-2` `markFlowRunComplete` now loads the run's steps and marks every non-terminal step DONE (falling back to the hub node if the load fails), then marks the run DONE (`flow_step_runtime.go`).

## 8. Validation

- `V-1` **(done)** `TestMarkFlowRunCompleteSettlesEveryStep`: from an inconsistent mid-finalization state (a reviewer + synthesis still RUNNING), `markFlowRunComplete` settles every step to DONE. Passes.
- `V-2` **(done)** No regression: broad flow/cohort/review subset → 197 passed. `go build` clean.
- `V-3` **(not executed)** A live review-loop confirming the timeline no longer shows synthesis DONE-early or stuck-RUNNING — no runner + provider account in this environment; being a timing race, the user should confirm across a few runs.

## 9. Regression Guard

- tests: `TestMarkFlowRunCompleteSettlesEveryStep`; existing cohort-join/`maybeAutoReinvokeHub` and BUG-179 advertisement-gate tests guard the surrounding order.
- alerts: none.
- audit checks: recorded in `change-audit/CA-219-order-cohort-join-step-writes-and-authoritative-finalize.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the step writes remain best-effort against the store; ordering + authoritative settle make the observable final state correct regardless.
