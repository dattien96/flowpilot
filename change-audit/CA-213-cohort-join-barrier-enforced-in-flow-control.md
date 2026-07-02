# CA-213: Cohort Join Barrier Enforced In Flow Control

## Summary

Fixed `BUG-176`: a review-loop's "synthesis" (hub) node could be marked DONE and the run terminated while both reviewers were still RUNNING, because `turnBridge.SubmitFlowControl` routed a child's flow-control call straight to the parent's `applyFlowControl` with no cohort-join gate. A single reviewer calling `submit_review_outcome` therefore short-circuited the whole round. Now only the hub's own post-join synthesis turn may drive the flow.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: `turnBridge.SubmitFlowControl` rejects a call from a cohort-member run (`flowCohortId != ""`) with instructive guidance to report findings in the final message; the hub (parent run, `flowCohortId == ""`) and non-cohort children are unaffected.
- `apps/local-runner/internal/runner/flow_step_runtime_test.go`: added `TestSubmitFlowControlRejectsCohortMemberButAllowsHub`.

## Verification

- New barrier test passes; `go test -run 'Flow|Workflow|Orchestrat|Cohort|Coder|Reviewer|Progress|StepRuntime|Advance|Resolve|SubmitFlowControl|Hub|Review|Loop'` → 314 passed (no regression in cohort-join, continue-reinvoke, or hub-advertisement tests). `go build ./internal/runner/` clean.
- Not verified live (no runner + provider account) — flagged in `BUG-176` (`V-3`).

## Notes

- Execution-layer backstop to the existing hub-only tool-advertisement gate (BUG-NOTE-CP42 #24) and the reviewer auto-spawn prompt (BUG-NOTE-CP42 #13); neither could prevent a model from invoking the tool anyway.
- Surfaced by BUG-174's honest step timeline. Sibling observation #5 (reviewer-cohort approvals not surfacing on the main run) is a separate item under investigation.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-176
change_type: bugfix
summary: Enforce the cohort join barrier in turnBridge.SubmitFlowControl so a reviewer cohort member can no longer terminate/advance the flow before the hub synthesizes after the join
# --->8---
