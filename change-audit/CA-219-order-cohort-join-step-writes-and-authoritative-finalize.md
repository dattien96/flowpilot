# CA-219: Order Cohort-Join Step Writes And Make Finalization Authoritative

## Summary

Fixed `BUG-181`: the synthesis step could show DONE while reviewers were still RUNNING, or stay stuck RUNNING after the flow completed. Cause was a race — the cohort-join step writes and the hub reinvoke (which finalizes via `markFlowRunComplete`) were dispatched in two independent goroutines with no ordering. Now the writes run before the reinvoke in one goroutine, and finalization settles every step DONE.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: the `EventMessageCompleted` cohort-join branch now runs the reviewer-DONE + synthesis-RUNNING step writes and then `maybeAutoReinvokeHub` in a single ordered goroutine, so the synthesis turn's finalization can't precede them.
- `apps/local-runner/internal/runner/flow_step_runtime.go`: `markFlowRunComplete` now loads the run's steps and marks every non-terminal step DONE (fallback: the inline hub node) before marking the run DONE — authoritative terminal settle.
- `apps/local-runner/internal/runner/flow_step_runtime_test.go`: added `TestMarkFlowRunCompleteSettlesEveryStep`.

## Verification

- New test passes; broad flow/cohort/review subset → 197 passed. `go build` clean.
- Not verified live (no runner + provider account); being a timing race, the user should confirm across a few runs — flagged in `BUG-181` (`V-3`).

## Notes

- Scoped to flowEngineDriven runs; no change to orchestration/loop semantics, only the ordering of step-runtime writes and the terminal settle.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-181
change_type: bugfix
summary: Sequence cohort-join step writes before the hub reinvoke and make markFlowRunComplete settle every step DONE, fixing synthesis showing done-early or stuck-running due to a goroutine race
# --->8---
