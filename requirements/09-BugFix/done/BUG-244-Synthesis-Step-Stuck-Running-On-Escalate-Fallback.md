# BUG-244: Synthesis Step Reads RUNNING After Escalate Because Loop-Blocked Flips Before Step Settles

## Metadata

- Document ID: `BUG-244`
- Title: `Synthesis Step Reads RUNNING After Escalate Because Loop-Blocked Flips Before Step Settles`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: [BUG-242: Coder Re-Entry Miscategorized; Synthesis Step Stuck Running](../done/BUG-242-Coder-Reentry-Miscategorized-Closed-And-Synthesis-Step-Stuck-Running.md), [BUG-233: Flow Timeline Staleness And Blocked-Card Diagnostic Content](../done/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md), [BUG-231](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md)
- Child Documents: `none`
- Related Documents: `change-audit/CA-243-escalate-settle-step-before-loop-blocked.md`
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, chat-mode, step-timeline, escalate, regression`

## AI Quick View

### Summary

- During CP-41 chat-mode E2E prep, `TestE2EReviewLoopSynthesisFallbackEscalates` was the one remaining failing review-loop test: after the hub synthesis turn completes in prose only (never calling `submit_review_outcome`), the escalate fallback fires and the loop correctly reaches `blocked` with the reviewers' findings — but the synthesis (hub inline) step was read as `RUNNING` instead of `WAITING_USER_APPROVAL`.
- Root cause (traced with a temporary step-transition log): the writes were in the RIGHT order (`RUNNING` then `WAITING_USER_APPROVAL` was the last write), but `applyFlowControl`'s `escalate` branch flipped the orchestrator loop to `blocked` (`mutateLoop`, in-memory, no SSE) BEFORE settling the step to `WAITING_USER_APPROVAL`. Any observer that keys off the loop status alone — the test's `waitLoop(loop.Status=="blocked")`, and in principle any poller reading orchestrator loop state — could read `blocked` in the window before the step settle landed.
- Production's normal desktop path is actually consistent (the only SSE emit is `emitAgentGraph`, which runs after the step settle), so the user-visible impact is limited to observers that read the in-memory loop state directly ahead of the SSE. But the ordering is still wrong causally and made the test racy.
- Fix: settle the step (`setFlowStepAwaitingUser`) BEFORE `mutateLoop` flips the loop to `blocked`, so "loop blocked" implies "step already settled" for every observer. Same escalate branch, reordered two lines.

### Current Ask

- Make the synthesis step reliably read `WAITING_USER_APPROVAL` the moment the loop is observably `blocked` on an escalate, closing the last failing review-loop test so Review-Loop-in-chat is clean for the manual E2E session.

### Key Decisions

- `D-1` This is a production-ordering fix, not a test change: the step reaching its awaiting-user state is the CAUSE and the loop being blocked is the EFFECT, so the step must settle first. The previously-failing `TestE2EReviewLoopSynthesisFallbackEscalates` now passes unmodified and serves as the regression guard (its assertion was correct; only its `waitLoop` barrier had been racing the production mis-ordering).

### Constraints

- Reorder only the `escalate` branch; do not touch the `continue`/cap-reached awaiting-user path (its test passes and its `mutateLoop` is more complex). The `emitAgentGraph` still runs last (BUG-233 settle-before-emit preserved).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` `applyFlowControl` "escalate" case — `setFlowStepAwaitingUser` now precedes `mutateLoop`.
- `apps/local-runner/internal/runner/agent_orchestrator.go` `mutateLoop` — confirmed it only updates in-memory loop state and returns a snapshot; it emits no SSE, which is why an observer reading loop state directly could beat the step settle.
- `apps/local-runner/internal/runner/interactive_service_e2e_test.go` `TestE2EReviewLoopSynthesisFallbackEscalates` — the regression test (now green).

## 1. Issue Summary

After a prose-only hub synthesis turn triggers the escalate fallback, the review loop pauses `blocked` with findings, but the synthesis step timeline row could read `RUNNING` instead of `WAITING_USER_APPROVAL` for any observer that reacts to the loop status before the step settle landed — because the escalate branch flipped the loop to blocked before settling the step.

## 2. Parent Links

- Surfaced during CP-41 chat-mode E2E readiness prep; same class as BUG-242 (stale step badge on a state transition) and BUG-233 (settle-before-emit ordering).

## 3. Environment and Reproduction

- environment: built-in Review Loop, chat bug sub-mode or flow mode; hub synthesis turn answers in prose without calling `submit_review_outcome`.
- reproduction: run `go test ./internal/runner/ -run TestE2EReviewLoopSynthesisFallbackEscalates` — before the fix it fails at the synthesis-step-status assertion; after, it passes.
- frequency: deterministic for an observer keying off loop status; the shipped desktop SSE path was already consistent.

## 4. Expected vs Actual

- expected: when the loop is observably `blocked` on escalate, the synthesis step already reads `WAITING_USER_APPROVAL`.
- actual: the loop flipped to `blocked` first; the step could still read `RUNNING` in the window before the settle.

## 5. Impact

- users affected: primarily automated/observer code reading the orchestrator loop state directly (the failing test); the normal SSE-driven desktop path was consistent.
- severity: low functionally (loop pauses and surfaces findings correctly); it was the last failing review-loop test and a real causal-ordering wrong-ness worth closing before the E2E session.

## 6. Root Cause

- confirmed cause: `applyFlowControl` "escalate" did `mutateLoop(Status=blocked)` before `setFlowStepAwaitingUser`. `mutateLoop` mutates in-memory loop state with no SSE, so the loop read `blocked` before the step settle. Verified with a temporary `setFlowStepStatus` log showing the final step write was `WAITING_USER_APPROVAL`, i.e. the writes were correctly ordered but the loop-status flip preceded the settle.

## 7. Fix Strategy

- `F-1` In the `escalate` branch, call `setFlowStepAwaitingUser` BEFORE `mutateLoop`. `emitAgentGraph(snap)` still runs last, preserving BUG-233's settle-before-emit for the SSE path.

## 8. Validation

- `V-1` `go build ./...` — clean.
- `V-2` `TestE2EReviewLoopSynthesisFallbackEscalates` — now PASS (was the sole failing review-loop test).
- `V-3` Full `TestE2EReviewLoop` suite — all green (ApprovedPath, MultiRound*, BlockedByProseOnly, ChangesRequested, CoderReentry (BUG-242), CapHitBlocked, EscalatePath, ExtendCap).
- `V-4` Full `go test ./internal/runner/` — 13 pre-existing environment failures only (Codex-CLI/Drive/skills-merge/interactive-auth), identical to the established baseline minus this now-fixed test; no new failures.

## 9. Regression Guard

- tests: `TestE2EReviewLoopSynthesisFallbackEscalates` (previously failing, now green with the fix).

## 10. Follow-Up Document Updates

- none — contained ordering fix, no contract/schema change.
