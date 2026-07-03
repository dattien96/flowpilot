# CA-235: Synchronous Step-Status Settlement + Reviewer-Findings Fallback Content

## Summary

BUG-233 tracked two issues surfaced by live testing of BUG-231's awaiting-user card: (1) the step timeline could show a stale "all done" snapshot while a reviewer child agent was still running, and (2) the blocked card could show an internal engine diagnostic sentence instead of the reviewers' actual findings. Both are fixed, entirely on the Go runner side — bug #1 turned out to be a server-side concurrency ordering bug, not the client-side dual-refresh race originally suspected, so no desktop changes were needed.

## What Changed

### Runner (`apps/local-runner`)

- `interactive_service.go` — **bug #1 (synchronous step-status settlement)**: every step-status write on a round-boundary transition (`applyFlowControl`'s "looping"/"awaiting_user" branches of the `continue` case, the "escalate" case, `resumeFlowWithFeedback`, and the cohort-join reviewer-DONE/hub-RUNNING write inside `emitLocked`) was dispatched inside a `go func(){...}()`, then the code called `emitAgentGraph` (the SSE event the desktop reacts to by refreshing the step-runtime snapshot) without waiting for that goroutine. Moved all four write sites to run synchronously before `emitAgentGraph`/before spawning the goroutine that also does other async work — `workflowStore.ApplyStepTransition` uses its own independent lock, so this is safe even inside `emitLocked` (which already holds `InteractiveService.mu`).
- `interactive_service.go` — **bug #4 (reviewer-findings fallback content)**: new `interactiveRun.lastCohortNote` field, populated at both cohort-join sites right after `buildCohortNote` runs (so it survives `pendingAgentContext` being drained into the hub's synthesis turn). New `lastCohortNoteFor(runID)` getter and `summarizeCohortNoteForUser(note)` helper (strips the note's engine-internal header/instructions, keeps only the per-reviewer findings lines, truncated to 800 chars). The CA-226 no-outcome-call fallback now builds its `GateReason`-bound `Summary` from the reviewers' findings when available, falling back to the original diagnostic sentence only when no cohort note exists.
- `interactive_service.go` — also fixed, same code path: the CA-226 fallback was unconditionally setting the hub node to `StepStatusFailed` right after `applyFlowControl(escalate)` had already settled it to `WAITING_USER_APPROVAL` per BUG-231 — an oversight predating BUG-231 that silently reverted its fix for this exact scenario. Removed.
- Tests: `TestApplyFlowControlEscalateSettlesHubToWaitingUser` / `TestApplyFlowControlCapReachedSettlesHubToWaitingUser` (tightened from eventual `waitLoop` to immediate assertions), `TestApplyFlowControlLoopingResetsStepsSynchronously` (new), `TestResumeFlowWithFeedbackAutoExtendsOnlyForCap`'s cap sub-test (extended with an immediate hub-status assertion), `TestE2EReviewLoopSynthesisFallbackEscalates` (updated to assert findings-based `GateReason` + `WAITING_USER_APPROVAL`, not `FAILED`), `TestSummarizeCohortNoteForUserStripsEngineInstructions`, `TestSummarizeCohortNoteForUserEmptyInputReturnsEmpty`.

### Docs

- `requirements/09-BugFix/done/BUG-233-...md`: investigation, root cause (revised — server-side, not the originally-suspected client-side race), fix strategy, Definition of Done, and code change plan; moved from `todo/` to `done/`.
- `requirements/09-BugFix/done/BUG-231-...md` §13: updated the two deferred items to "Fixed in BUG-233".

## Verification

- `go build ./...` / `go vet ./internal/runner/...` — clean.
- `go test ./internal/runner/...` — 1068 passed, 15 failed (identical pre-existing baseline from BUG-231/CA-233 — Windows-path/Codex-CLI/Google-Drive environment gaps), 14 skipped.
- New/updated tests re-run with `-count=10` (200 runs total) — 0 failures, ruling out flakiness in the newly-synchronous assertions.
- Not executed: a live click-through in the running desktop app (same sandbox limitation as CA-233/CA-234). No desktop code changed for this fix.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-233
change_type: bugfix
summary: Make flow step-status settlement synchronous to eliminate a stale-timeline race, and show reviewers' actual findings instead of an internal diagnostic sentence on the CA-226 no-outcome fallback
# --->8---
