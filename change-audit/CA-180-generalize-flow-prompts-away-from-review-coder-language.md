# CA-180: Generic Flow Prompts No Longer Hardcode Review/Coder Language (BUG-NOTE-CP42 #26)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: two of the flow engine's built-in prompts (and the Go fallback strings that mirror them) were written assuming every flow is Review Loop, even though CP-42's whole point is that flow behavior is generic/pack-driven.

## The bug

- `prompts/flow-start-wait.md` — shown to the hub of **any** flow that auto-spawns an entry node (`notifyHubFlowStarted`) — said "A coder agent has already been spawned... You are the review coordinator... wait... a review cycle completes." For `rag-harness` (whose hub is never reviewing anything — it's coordinating a plan→code→test→audit pipeline), this is actively misleading.
- `prompts/coder-reentry.md` — shown on every back-edge "continue" reentry (`maybeReinvokeCoderForContinue`) — said "Review completed with requested changes... resubmit the code." `rag-harness.yaml` has a `validate -> implement` continue back-edge for command-validation failures, not review feedback; the same review-flavored text would show there too, once Task-180's edge-driven back-edge resolution actually routes a `rag-harness` continue signal correctly (it does, per CA-161).
- Several Go fallback strings (used only when the corresponding pack `.md` file can't be loaded) independently hardcoded the same review/coder-specific wording, so even fixing the pack files alone would have left an inconsistent fallback.

`prompts/auto-reinvoke.md` was checked and found already domain-free — it says "call the declared control tool for this flow when one is available," naming `submit_review_outcome` only as a specific example for "the built-in review loop." No change needed there; only its Go fallback string in `autoReinvokePromptText` needed the same generalization for consistency.

## Fix

- `flow-start-wait.md`: "An agent has already been spawned to work on this request. Do not duplicate that work yourself — wait for its result; you will be reinvoked automatically once this step of the flow completes."
- `coder-reentry.md`: "Feedback received on your last submission.\n\nAddress every issue below and resubmit your work."
- Matching Go fallback strings updated in `flow_executor.go` (`notifyHubFlowStarted`), `interactive_service.go` (`startTurn`'s inline wait-notice fallback, `autoReinvokePromptText`, `buildCoderReentryPrompt`'s empty-content fallback, `maybeReinvokeCoderForContinue`'s empty-prompt fallback).

## Verification

- Updated pre-existing tests that had encoded the old wording as their expected content: `TestLoadBuiltinCoderReentryPrompt` (agentpack), `TestStartResolvedFlowNotifiesHubToWait` and `TestStartTurnWithFlowRefPrependsWaitNoticeToHubsOwnFirstTurn` (runner) — all now check for the new generic phrasing instead.
- New test `TestFlowStartWaitPromptIsDomainFree` (`pack_test.go`): asserts `flow-start-wait.md`'s content contains none of "coder", "review coordinator", "review cycle" (case-insensitive).
- Full `internal/agentpack` (14 tests) and `internal/runner` (1009 passed, 15 pre-existing/environmental failures unchanged) suites pass.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: reword flow-start-wait.md and coder-reentry.md (plus their Go fallback strings) to flow-agnostic language, so a non-review flow like rag-harness no longer shows review/coder-specific prompts on its hub-wait and continue-reentry paths
# --->8---
