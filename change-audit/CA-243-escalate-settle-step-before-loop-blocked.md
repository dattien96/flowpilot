# CA-243: Settle Synthesis Step Before Flipping Loop To Blocked On Escalate

## Summary

BUG-244: the last failing review-loop test (`TestE2EReviewLoopSynthesisFallbackEscalates`) showed the synthesis (hub inline) step reading `RUNNING` instead of `WAITING_USER_APPROVAL` after a prose-only hub synthesis turn triggered the escalate fallback. Root cause: `applyFlowControl`'s `escalate` branch flipped the orchestrator loop to `blocked` (`mutateLoop`, in-memory, no SSE) BEFORE settling the step, so an observer keying off the loop status (the test's `waitLoop`, or any direct loop-state reader) could see `blocked` before the step settle landed. The shipped desktop SSE path was already consistent (the only emit runs after the settle), so impact was limited, but the causal ordering was wrong.

## What Changed

### Runner (`apps/local-runner`)

- `interactive_service.go` `applyFlowControl` "escalate" case: moved `setFlowStepAwaitingUser` to run BEFORE `mutateLoop(Status=blocked)`, so the step reaches `WAITING_USER_APPROVAL` before the loop is observably `blocked`. The step settling is the cause; the loop being blocked is the effect. `emitAgentGraph(snap)` still runs last, preserving BUG-233's settle-before-emit ordering for the SSE-driven desktop refresh.

### Tests

- `TestE2EReviewLoopSynthesisFallbackEscalates` — previously the sole failing review-loop test; now passes unmodified and serves as the regression guard (its assertion was always correct; only its `waitLoop(loop.Status=="blocked")` barrier had been racing the production mis-ordering).

### Docs

- `requirements/09-BugFix/done/BUG-244-Synthesis-Step-Stuck-Running-On-Escalate-Fallback.md`.

## Verification

- `go build ./...` — clean.
- `TestE2EReviewLoopSynthesisFallbackEscalates` — PASS; full `TestE2EReviewLoop` suite — all green.
- Full `go test ./internal/runner/`: 13 pre-existing environment-specific failures only (Codex-CLI resume, Google Drive, skills-merge home dirs, interactive auth), identical to the established branch baseline with the escalate test removed from the failing set. No new failures.
- Diagnosis used a temporary `setFlowStepStatus` debug log (since reverted) that confirmed the final step write was `WAITING_USER_APPROVAL` — i.e. writes were correctly ordered but the loop-status flip preceded the settle.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-244
change_type: bugfix
summary: On escalate, settle the synthesis step to WAITING_USER_APPROVAL before flipping the loop to blocked so loop-blocked implies step-settled for every observer
# --->8---
