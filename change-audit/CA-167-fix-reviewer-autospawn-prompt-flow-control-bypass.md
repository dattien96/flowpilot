# CA-167: Fix Reviewer Auto-Spawn Prompt Bypassing the Cohort Join Barrier (BUG-NOTE-CP42 #13)

## Scope

Verified and fixed a P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`, introduced by this session's own earlier CA-163 change (the forward-edge auto-spawn feature) rather than a pre-existing defect.

## The bug

CA-163's `tryAdvanceFlowFromNode` (`flow_executor.go`) prompted an auto-spawned reviewer with: *"report your findings via the flow's declared control tool."* `turnBridge.SubmitFlowControl` (`interactive_service.go`) routes any child run's flow_control tool call straight to `applyFlowControl(parentRunID, ...)` — mutating the parent hub's loop state (round increment, cap check, status transitions) — with **no cohort-join gate at all**. If a reviewer literally followed that instruction, it could advance/complete/block the whole flow round before the other cohort member(s) even finished, bypassing the join barrier `maybeAutoReinvokeHub` depends on entirely — the opposite of Task-091 T-3's "reviewer only contributes to cohort note" design.

Every other reviewer-spawn path in this codebase gets it right: the manually-spawned E2E tests prompt reviewers with just "review the coder output," and the built-in `reviewer` agent persona (`agent_catalog.go`) only asks for `APPROVED`/`CHANGES-REQUESTED` text feedback — it never mentions `submit_review_outcome`. Only the `synthesizer` persona is instructed to call that tool, and its own system prompt explicitly scopes that to *after* consolidating the cohort join note. CA-163's auto-spawn prompt was the one place in the codebase that got this wrong.

## Fix

Reworded the auto-spawn prompt to match the established convention: ask the reviewer to review and report its findings (approve or request changes, with specifics) as its own final message, with no instruction to call any tool. This relies on the existing, correct design — the reviewer's final message becomes part of the cohort join note (`buildCohortNote`), and only the hub's own synthesis turn (after `maybeAutoReinvokeHub` fires post-join) calls the control tool.

## Verification

- New test `TestCoderCompletionAutoSpawnedReviewerPromptDoesNotInstructFlowControlCall` (`flow_executor_test.go`): drives `startResolvedFlow` end to end and asserts the auto-spawned reviewer's observed prompt contains none of "control tool", "flow_control", or "submit_review_outcome".
- Full flow-related test group (61 tests) passes unchanged.
- `go build ./...` clean.

## Note

No defense-in-depth gate was added to `applyFlowControl`/`turnBridge.SubmitFlowControl` itself (e.g. rejecting a flow_control call from a run that is a still-open cohort member) — the fix here removes the only thing that was actually instructing a reviewer to make that call. A future custom user-authored flow with a delegate node whose own `promptTemplate` independently instructs the same thing would still hit this gap; that's a broader hardening question left open, not something this specific regression required.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: reword the forward-edge auto-spawned reviewer's prompt so it reports findings as its own message instead of being told to call the flow's control tool directly, which bypassed the cohort join barrier
# --->8---
