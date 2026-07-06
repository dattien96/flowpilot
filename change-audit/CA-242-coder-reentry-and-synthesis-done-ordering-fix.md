# CA-242: Unify Flow-Child Reinvoke Path And Settle Steps Before Emitting On Flow Done

## Summary

BUG-242: live testing of the Review Loop across rounds surfaced two desktop display bugs. (1) A round-2+ coder re-entry via the synthesis→coder back-edge stayed miscategorized in "Recently closed" with no new main-chat card, because `maybeReinvokeCoderForContinue`'s reuse branch was a second, older inline reinvoke implementation that never got the BUG-Rnd2 `activationSeq`/`EventAgentSpawnedByUser` fixes already present in the separate `reinvokeExistingFlowChild` (forward-edge) path. (2) The synthesis/hub step displayed RUNNING after the flow reached `done` until an unrelated focus switch corrected it, because `applyFlowControl`'s "done" case emitted the desktop-facing `agent_graph_updated` event before `markFlowRunComplete` settled the step timeline — the same ordering bug BUG-233 had already fixed for the "continue" branch. A third observation (reviewers re-spawning as fresh cards each round) was confirmed correct, not a bug.

## What Changed

### Runner (`apps/local-runner`)

- `flow_executor.go`: extracted `reinvokeMatchingFlowChild(parentRunID, prompt, match)` — the single implementation of "find a non-turnInFlight child matching `match`, increment `activationSeq`, set running, upsert its summary, emit the agent-graph snapshot, emit `EventAgentSpawnedByUser`, schedule the next turn." `reinvokeExistingFlowChild` now delegates to it (exact node-id match).
- `interactive_service.go` `maybeReinvokeCoderForContinue`: its reuse branch now delegates to `reinvokeMatchingFlowChild` instead of a duplicated, older inline copy — the actual code path a review-loop round-2+ coder re-entry takes.
- `interactive_service.go`: two other `AgentRunSummary` reconstructions (the generic per-event summary upsert, and `releaseDependentAgents`'s queued-turn summary) now preserve `ActivationSeq` from the live `interactiveRun` instead of omitting it — previously the very next turn-progress event after a reinvoke silently reset the reported counter back to 0, undermining the fix above under realistic timing.
- `interactive_service.go` `applyFlowControl`'s "done" case: `markFlowRunComplete` now runs before `emitAgentGraph`, matching the ordering the "continue"/looping branch already had (BUG-233), so the desktop's SSE-triggered step-runtime refresh never races ahead of the DONE writes.

### Tests

- `TestE2EReviewLoopCoderReentryIncrementsActivationSeqAndEmitsSpawnEvent` (interactive_service_e2e_test.go) — drives a real back-edge continue-reinvoke and asserts activationSeq increases and a spawn event lands.
- `TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph` (flow_step_runtime_test.go) — wraps the workflow store to prove the hub node's step is already DONE by the time the "done" agent-graph event would be observed; verified against a temporary revert to confirm it actually catches the regression.

### Docs

- `requirements/09-BugFix/done/BUG-242-Coder-Reentry-Miscategorized-Closed-And-Synthesis-Step-Stuck-Running.md`.

## Verification

- `go build ./...` (apps/local-runner) — clean.
- Both new tests green; confirmed `TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph` fails when the old (pre-fix) emit-then-settle order is temporarily restored, and passes again once reverted back.
- Existing review-loop E2E suite still green (approved path, multi-round changes-then-approved, multi-round slow synthesis, cap-hit-blocked, escalate, reseed/step-status/advance-flow unit tests).
- Full `go test ./internal/runner/...`: identical 14 pre-existing failures to the established branch baseline (Codex-CLI/Google-Drive/skills-merge-home-dir/interactive-auth environment tests, plus `TestE2EReviewLoopSynthesisFallbackEscalates` — confirmed via a `git stash` before/after comparison to be unrelated pre-existing WIP in the step-status escalation path, unaffected by this change). No new failures.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-242
change_type: bugfix
summary: Unify the two divergent flow-child reinvoke implementations behind one activationSeq/spawn-event-correct path, and settle the step timeline before emitting on flow done, so the desktop never shows a stale closed/running status
# --->8---
