# BUG-179: Hub Finalizes The Flow On Its First Turn, Before The Review Cohort Runs

## Metadata

- Document ID: `BUG-179`
- Title: `Hub Finalizes The Flow On Its First Turn, Before The Review Cohort Runs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-176-Cohort-Reviewer-Can-Terminate-Flow-Before-Join-Barrier.md`, `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`
- Replaces: `none`
- Tags: `agent-flow-engine, runner, flow-mode, review-loop, orchestration, join-barrier`

## AI Quick View

### Summary

- The step timeline showed the "synthesis" (hub) node DONE and the run completed while both reviewer nodes were still RUNNING — the hub must wait for the whole review cohort, then synthesize.
- BUG-176 blocked a cohort *reviewer* from driving flow control, but the offender here is the **hub itself**. `startResolvedFlow` sets the hub's `autoOrchestrate = true` as soon as it spawns the entry coder (from a goroutine), and tool advertisement gated solely on `autoOrchestrate` — so the hub's **first** model turn was offered `submit_review_outcome` and could call `flow_control("done")` before the cohort had even run. The hub has `flowCohortId == ""`, so BUG-176's guard doesn't stop it, and `applyFlowControl` has no cohort/turn gate.
- Fix: offer the flow control tool only on the hub's genuine synthesis turn — `autoOrchestrate && parentRunID == "" && turnCount > 1` — so it is withheld from the first turn (and from every child), leaving only the post-cohort-join reinvoke turn able to finalize.

### Current Ask

- Only the hub's post-join synthesis turn may finalize the flow; no run may mark synthesis/done while the review cohort is still running.

### Key Decisions

- `V-1` `offerReviewOutcomeTool` requires the run be the hub (`parentRunID == ""`), auto-orchestrating, and past its first turn (`turnCount > 1`). The coder spawns on turn 1; `maybeAutoReinvokeHub` only reinvokes the hub (turn ≥ 2) after `cohortComplete`, so `turnCount > 1` is the real synthesis turn.

### Constraints

- Runner-only, one-line predicate change; the hub's synthesis turn still gets the tool. Children (coder, reviewers) never get it advertised (`parentRunID != ""`), complementing BUG-176's `flowCohortId` guard.
- In practice the Flow-mode composer is disabled mid-run ("Waiting for the current turn…"), so the hub does not take an unrelated turn between its first turn and the synthesis reinvoke — `turnCount > 1` reliably identifies synthesis.

### Open Questions

- Defense-in-depth: `applyFlowControl("done"/"continue")` could additionally reject finalization while any registered cohort is incomplete. Deferred — it risks wrongly blocking a legitimate synthesis-turn finalize if the "pending cohort" state is misjudged, and could not be verified live here; the advertisement gate is the confirmed, low-risk root-cause fix.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go:2267` (`offerReviewOutcomeTool` — pre-fix gated on `autoOrchestrate` alone)
- `apps/local-runner/internal/runner/interactive_service.go:2687` (`turnCount++` in `startTurn`, before `go s.runTurn`)
- `apps/local-runner/internal/runner/interactive_service.go:1937-1940` (`spawnChildRun` sets the hub's `autoOrchestrate = true`)
- `apps/local-runner/internal/runner/interactive_service.go:1793-1799` (BUG-176 guard — blocks cohort members only, not the hub)
- `apps/local-runner/internal/runner/interactive_service.go` (`applyFlowControl` "done"/"continue" — no cohort/turn gate)

## 1. Issue Summary

In a review-loop, the hub's first model turn was offered the `submit_review_outcome` (flow control) tool and used it to finalize the flow before the reviewer cohort ran, marking the synthesis node DONE and the run complete while the reviewers were still RUNNING.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: local runner, a Flow-Mode review-loop, YOLO off.
- reproduction steps:
  1. Start a review-loop; the hub spawns the coder.
  2. On its first turn the hub is offered `submit_review_outcome` and calls it ("approved" → done).
  3. Observe synthesis marked DONE and the run completed while reviewer_correctness / reviewer_security are still RUNNING.
- frequency: reproducible; the hub's first turn had the tool.

## 4. Expected vs Actual

- expected: the hub waits, the coder runs, the reviewer cohort runs and joins, then the hub's synthesis turn finalizes.
- actual: the hub finalized on its first turn, before the cohort ran.

## 5. Impact

- users affected: anyone running a review-loop (or any cohort flow).
- workflows affected: the review is skipped and the run is reported complete prematurely.
- severity: high — the core review loop is bypassed and the timeline misreports completion.

## 6. Root Cause

- confirmed cause: `startResolvedFlow` → `spawnChildRun` sets the hub's `autoOrchestrate = true` (`interactive_service.go:1937-1940`) from a goroutine started on the hub's first turn. `runTurn` computed `offerReviewOutcomeTool := rs.autoOrchestrate` (`:2267`), so the hub's first turn advertised the tool. The hub (`flowCohortId == ""`) is not blocked by BUG-176's `SubmitFlowControl` guard (`:1793-1799`), and `applyFlowControl("done")` has no cohort/turn gate — so the first-turn call finalized the flow immediately. "Hub turns only" (BUG-NOTE-CP42 #24) had never been implemented as *post-join only*.

## 7. Fix Strategy

- `F-1` Gate the advertisement: `offerReviewOutcomeTool := rs.autoOrchestrate && rs.parentRunID == "" && rs.turnCount > 1` (`interactive_service.go:2267`). The tool is offered only to the hub, and only from its synthesis turn (turn ≥ 2, reached via `maybeAutoReinvokeHub` after the cohort join) — never its first turn and never to a child.

## 8. Validation

- `V-1` **(done)** `TestReviewOutcomeToolOfferedOnlyOnHubSynthesisTurn` (`flow_step_runtime_test.go`): a fake adapter captures `TurnRequest.OfferReviewOutcomeTool` — false on the hub's first turn, true on its second (synthesis) turn. Passes.
- `V-2` **(done)** No regression: the MCP-advertisement tests (`submit_review_outcome` for a hub turn / rejected for non-hub) still pass; broad flow/orchestration subset → 278 passed. `go build` clean.
- `V-3` **(not executed)** A live review-loop confirming the hub now waits for both reviewers before synthesis/finalization — no runner + provider account in this environment.

## 9. Regression Guard

- tests: `TestReviewOutcomeToolOfferedOnlyOnHubSynthesisTurn`; BUG-176's `TestSubmitFlowControlRejectsCohortMemberButAllowsHub` guards the cohort-member path; existing MCP-advertisement tests guard the hub/non-hub advertisement.
- alerts: none.
- audit checks: recorded in `change-audit/CA-217-hub-review-tool-only-on-synthesis-turn.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — enforces CP-36's hub-drives-transitions-after-join design.
- notes left unchanged on purpose: the defense-in-depth `applyFlowControl` cohort gate is deferred (see Open Questions); the advertisement gate is sufficient for the realistic (composer-disabled mid-run) flow.
