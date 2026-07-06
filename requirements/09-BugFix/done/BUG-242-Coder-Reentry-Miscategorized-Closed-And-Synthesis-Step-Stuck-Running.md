# BUG-242: Coder Re-Entry Miscategorized As Closed; Synthesis Step Stuck RUNNING Until Focus Switch

## Metadata

- Document ID: `BUG-242`
- Title: `Coder Re-Entry Miscategorized As Closed; Synthesis Step Stuck RUNNING Until Focus Switch`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: [BUG-235: Stale Child Agent Run Status After Flow Done](../done/BUG-235-Stale-Child-Run-Status-And-Flow-Engine-Prompt-Overwrites-History-Title.md), [BUG-233: Flow Timeline Staleness And Blocked-Card Diagnostic Content](../done/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md)
- Child Documents: `none`
- Related Documents: `change-audit/CA-242-coder-reentry-and-synthesis-done-ordering-fix.md`
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, agents-panel, desktop, regression`

## AI Quick View

### Summary

Live testing of the Review Loop across multiple rounds surfaced two more display bugs (a third, reviewer re-spawn, was confirmed correct behavior — not a bug):

1. **Coder round-2+ re-entry stayed in "Recently closed" with no new main-chat card** (fixed): the back-edge "continue" reinvoke (`maybeReinvokeCoderForContinue`) had its own older, un-fixed inline reinvoke logic — a separate copy of `reinvokeExistingFlowChild` (the forward-edge reuse path) that never got the BUG-Rnd2 fixes (incrementing `activationSeq`, emitting `EventAgentSpawnedByUser`). Without those, the desktop's monotonic terminal-status guard (`mergeAgentRunsById`) discarded the completed→running transition as a stale snapshot, and no new agent card was rendered.
2. **Reviewer re-spawn on each round creates a new RUNNING card + new main-chat line** (confirmed correct): reviewers are `lifecycle: spawn`, so a fresh child run per round is the intended behavior.
3. **Synthesis step stuck showing RUNNING after flow done, until an unrelated focus switch** (fixed): `applyFlowControl`'s "done" branch called `emitAgentGraph` (which triggers the desktop's `refreshWorkflowStepRuntime`) *before* `markFlowRunComplete` settled the step timeline to DONE — the exact ordering bug BUG-233 had already fixed for the "continue"/looping branch, but the "done" branch was never reordered.

A related data-integrity gap surfaced while regression-testing #1: the generic per-event summary writer (`interactive_service.go`, the code path every turn-progress event goes through) never propagated `activationSeq` into the upserted `AgentRunSummary`, so the very next event after a reinvoke (e.g. the reinvoked child's own completion) silently reset the reported `activationSeq` back to 0 — undermining the exact mechanism #1's fix depends on. Fixed at both of its call sites.

### Current Ask

- Both #1 and #3 are contained, root-caused fixes. #2 required no code change (confirmed correct).

### Key Decisions

- `D-1` (#1) Extract the two duplicated "reactivate an existing child" implementations into one shared `reinvokeMatchingFlowChild(parentRunID, prompt, match func(*interactiveRun) bool)`. `reinvokeExistingFlowChild` (forward-edge reuse, matches by exact node id) and `maybeReinvokeCoderForContinue`'s reuse branch (back-edge continue, matches by node id when tracked, else falls back to the legacy `isCoderRun` role match for AI-driven runs with no tracked flow edges) both delegate to it now — a single place owns the `activationSeq` increment and `EventAgentSpawnedByUser` emission, so neither path can silently regress the other's fix again.
- `D-2` (#1 follow-up) Preserve `activationSeq` in every `AgentRunSummary` reconstruction that copies fields off the live `interactiveRun` (the generic per-event summary write in `finalizeInputLocked`'s caller, and `releaseDependentAgents`'s queued-turn summary) — previously these two omitted the field entirely, defaulting it to 0 and quietly erasing a genuine reinvoke's counter on the next unrelated event.
- `D-3` (#3) Reorder `applyFlowControl`'s "done" case: call `markFlowRunComplete` (settles the step timeline + child runs) *before* `emitAgentGraph` (fires the SSE event the desktop reacts to by refreshing), matching the ordering BUG-233 already established for the "continue" branch.

### Constraints

- Scoped to display/status-sync correctness; no change to flow control semantics, cap/round logic, or the review-loop topology itself.
- The wider `task/flow-agents` working tree carries unrelated in-flight BUG-234/235 changes to the step-status escalation path; `TestE2EReviewLoopSynthesisFallbackEscalates` fails identically with and without this fix (confirmed via a before/after stash comparison) and is out of scope here.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/flow_executor.go` `reinvokeExistingFlowChild`, new `reinvokeMatchingFlowChild`.
- `apps/local-runner/internal/runner/interactive_service.go` `maybeReinvokeCoderForContinue`, `applyFlowControl` ("done" case), the generic per-event `AgentRunSummary` upsert, `releaseDependentAgents`.
- `apps/desktop-flowpilot/src/state/store.ts` `mergeAgentRunsById` (BUG-235/BUG-Rnd2 monotonic guard, consumer of `activationSeq`), `refreshWorkflowStepRuntime` (consumer of `agent_graph_updated`).
- Prior fix this builds on: BUG-Rnd2 (activationSeq / `EventAgentSpawnedByUser`, referenced in existing code comments — no standalone doc found; behavior confirmed via `reinvokeExistingFlowChild`'s pre-existing implementation and `store.test.ts`'s `mergeAgentRunsById ... BUG-Rnd2` test cases).
- BUG-233 (`../done/BUG-233-...md`) established the settle-before-emit ordering for the "continue" branch that #3 now applies to "done" too.

## 1. Issue Summary

Two desktop display bugs in the Review Loop's multi-round flow: (1) a round-2+ coder re-entry via the synthesis→coder back-edge stayed miscategorized in the Agents panel's "Recently closed" section with no new main-chat card, even though the backend had genuinely restarted it; (3) the synthesis/hub step stayed shown as RUNNING after the flow reached `done`, only correcting itself when the user happened to switch focus between agents (which triggers an unrelated refresh). A third observation (reviewers re-spawning as fresh cards each round) was investigated and confirmed to be correct, intended behavior.

## 2. Parent Links

- impacted coding plan: none identified (display/status-sync bugfix, no schema or contract change)
- impacted tech design: none identified
- impacted system spec: none identified

## 3. Environment and Reproduction

- environment: built-in Review Loop, multi-round (changes requested → coder re-entry → re-review), desktop Agents panel + step timeline.
- reproduction steps (#1): run a review loop to round 2 (reviewers request changes); observe the coder reappear as "running" in the middle step-timeline column but remain under "Recently closed" in the right Agents panel, with no new agent card in the main chat thread.
- reproduction steps (#3): run a review loop to completion (`done`); observe the synthesis step's badge stays RUNNING; switching focus to another agent and back corrects it to DONE.
- frequency: deterministic — #1 on every round-2+ coder re-entry via the back-edge; #3 on every flow completion.

## 4. Expected vs Actual

- #1 — expected: a genuinely reactivated coder run moves back to the RUNNING section and a new main-chat card appears for the turn, matching the forward-edge reviewer-reuse behavior. actual: it stayed in "Recently closed" with no new card.
- #3 — expected: once the flow's control tool reports `done`, every step (including the hub/synthesis node) reads DONE immediately. actual: synthesis stayed RUNNING until an unrelated event (agent focus switch) triggered a fresh refresh.

## 5. Impact

- #1: confusing/misleading agent status during exactly the round where the user most needs to see the coder actively working again; could read as "the loop is stuck."
- #3: a completed flow visually appears to still have a running step, which can worry a user checking whether the flow actually finished.
- severity: medium — cosmetic/status-sync only, no functional breakage of the review loop itself.

## 6. Root Cause

- #1 confirmed cause: `maybeReinvokeCoderForContinue`'s reuse branch (interactive_service.go) carried its own inline "find matching child, set running, upsert summary" logic — written before BUG-Rnd2 added `activationSeq` incrementing and `EventAgentSpawnedByUser` emission to the *other* reinvoke path, `reinvokeExistingFlowChild` (flow_executor.go). The two implementations diverged: only the forward-edge path got the fix. Since `activationSeq` never increased on this path, `mergeAgentRunsById`'s BUG-235 monotonic guard (`prev terminal && incoming non-terminal` requires a strictly higher `activationSeq` to allow the transition) discarded the completed→running update as a stale snapshot, and the missing spawn-event emission meant no new chat card.
- #1 follow-up confirmed cause: while writing the regression test, the fix's `activationSeq` increment was observed to not survive past the reinvoked child's own next turn-progress event. The generic per-event `AgentRunSummary` upsert (interactive_service.go, fired on every provider event for a child, e.g. `EventTurnCompleted`) reconstructs the summary from the live `interactiveRun` fields but never included `ActivationSeq` — so it always wrote 0, silently clobbering whatever value `reinvokeMatchingFlowChild` had just set moments earlier. `releaseDependentAgents`'s queued-turn summary had the identical omission.
- #3 confirmed cause: `applyFlowControl`'s "done" case called `s.emitAgentGraph(parentRunID, snap)` before `s.markFlowRunComplete(...)`. `emitAgentGraph` fires the `agent_graph_updated` SSE event the desktop reacts to by calling `refreshWorkflowStepRuntime`; `markFlowRunComplete`'s step-DONE writes (via `setFlowStepStatus`) emit no event of their own. So the desktop's triggered refresh could read the step store before the DONE writes landed, and — since `setFlowStepStatus` has no subsequent event to prompt a correction — the stale RUNNING read stuck until some unrelated event (e.g. a focus-switch-triggered refresh) happened to re-fetch after the writes had completed.

## 7. Fix Strategy

- `F-1` Add `reinvokeMatchingFlowChild(parentRunID, prompt, match)` in flow_executor.go: the single implementation of "find a non-turnInFlight child matching `match`, increment `activationSeq`, set running, upsert summary (with `ActivationSeq`), emit the agent-graph snapshot, emit `EventAgentSpawnedByUser`, schedule the next turn." `reinvokeExistingFlowChild` now delegates to it with an exact-node-id match closure.
- `F-2` Replace `maybeReinvokeCoderForContinue`'s duplicated reuse-branch logic with a call to `reinvokeMatchingFlowChild`, using the same match precedence it already had (exact node id when a back-edge target is tracked, else the legacy `isCoderRun` role match for untracked/AI-driven runs).
- `F-3` Add `ActivationSeq: rs.activationSeq` / `ActivationSeq: child.activationSeq` to the two other `AgentRunSummary` reconstructions that read live `interactiveRun` fields (the generic per-event upsert, and `releaseDependentAgents`), so the counter is never silently reset by an unrelated event.
- `F-4` Reorder `applyFlowControl`'s "done" case: call `markFlowRunComplete` before `emitAgentGraph`, matching the "continue" branch's existing BUG-233 ordering.

## 8. Validation

- `V-1` `go build ./...` (apps/local-runner) — clean.
- `V-2` New test `TestE2EReviewLoopCoderReentryIncrementsActivationSeqAndEmitsSpawnEvent` (interactive_service_e2e_test.go): drives a real `applyFlowControl("continue")` back-edge reinvoke and asserts (a) the coder's summary `ActivationSeq` strictly increases, and (b) an `EventAgentSpawnedByUser` for that run lands on the parent's timeline.
- `V-3` New test `TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph` (flow_step_runtime_test.go): wraps the workflow store to observe, at the instant the synthesis node's step is written to DONE, whether any "done" `agent_graph_updated` event has already been emitted — asserts zero. Confirmed by a before/after sanity check (reverting the ordering makes the test fail with exactly the read count you'd expect from the race; restoring it passes).
- `V-4` Existing regression suite still green: `TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt`, `TestE2EReviewLoopApprovedPath`, `TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes`, `TestE2EReviewLoopMultiRoundSlowSynthesisStillCompletes`, `TestE2EReviewLoopCapHitBlocked`, `TestE2EReviewLoopEscalatePath`, `TestReseedFlowStepRuntimeSeedsOneStepPerNode`, `TestSetFlowStepStatusTransitionsByNodeID`, `TestTryAdvanceFlowMarksCompletedDoneAndTargetsRunning`.
- `V-5` Full `go test ./internal/runner/...` shows the identical 14 pre-existing failures as the established branch baseline (Codex-CLI/Google-Drive/skills-merge-home-dir/interactive-auth environment tests, plus `TestE2EReviewLoopSynthesisFallbackEscalates` — confirmed via a `git stash` before/after comparison to fail identically with and without this change, so it is unrelated WIP in the step-status escalation path). No new failures introduced.

## 9. Regression Guard

- tests: `TestE2EReviewLoopCoderReentryIncrementsActivationSeqAndEmitsSpawnEvent`, `TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph`.
- alerts: none.
- audit checks: `change-audit/CA-242-*`; watch that any future "reactivate an existing child" code path is added by calling `reinvokeMatchingFlowChild`, not by writing a third inline copy.

## 10. Follow-Up Document Updates

- upstream docs: none required — desktop-visible display/status-sync fix, no contract or schema change.
- notes left unchanged on purpose: `TestE2EReviewLoopSynthesisFallbackEscalates`'s failure (unrelated pre-existing WIP in the escalate/step-status path) is left as-is for whichever change owns that in-flight work to resolve.
