# CA-217: Offer The Flow Control Tool Only On The Hub's Synthesis Turn

## Summary

Fixed `BUG-179`: the review-loop's synthesis node was marked DONE and the run completed while both reviewers were still RUNNING, because the hub's FIRST turn was offered `submit_review_outcome` and could call `flow_control("done")` before the cohort ran. `startResolvedFlow`'s coder spawn sets the hub's `autoOrchestrate = true`, and advertisement was gated on that alone. Now the tool is offered only on the hub's genuine synthesis turn.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: `offerReviewOutcomeTool` now requires `autoOrchestrate && parentRunID == "" && turnCount > 1` — hub only, past its first turn (the post-cohort-join reinvoke turn). Children never get it advertised, complementing BUG-176's cohort-member guard in `SubmitFlowControl`.
- `apps/local-runner/internal/runner/flow_step_runtime_test.go`: added `TestReviewOutcomeToolOfferedOnlyOnHubSynthesisTurn`.

## Verification

- New test passes (false on first turn, true on synthesis turn); MCP-advertisement tests still pass; broad flow/orchestration subset → 278 passed. `go build` clean.
- Not verified live (no runner + provider account) — flagged in `BUG-179` (`V-3`).

## Notes

- Defense-in-depth (a cohort gate in `applyFlowControl`) was considered and deferred — it risks wrongly blocking a legitimate synthesis finalize and can't be verified live; the advertisement gate is the confirmed root-cause fix.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-179
change_type: bugfix
summary: Offer the flow control tool only on the hub's post-join synthesis turn (autoOrchestrate + parent + turnCount>1) so the hub can no longer finalize the flow before the review cohort runs
# --->8---
