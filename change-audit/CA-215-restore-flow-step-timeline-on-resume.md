# CA-215: Restore Flow Step Timeline On Resume After Restart

## Summary

Fixed `BUG-178`: reopening a completed flow-mode run from history after a server restart showed "No step-runtime data for this run yet". The local runner keeps step-runtime state in memory (`fakeWorkflowStore`), so it's lost on restart, and `reconstructRun` re-seeded steps only for chat runs. The resume path now rebuilds a flow run's step list from the persisted flow nodes.

## What Changed

- `apps/local-runner/internal/runner/flow_step_runtime.go`: added `reseedFlowStepRuntimeForResume` (rebuild steps from nodes, DONE for a completed run else PENDING) and factored the shared `flowStepRowsFromNodes` builder out of `reseedFlowStepRuntime`.
- `apps/local-runner/internal/runner/interactive_resume.go`: `reconstructRun` now rebuilds the step list from `rs.activeFlowNodes` (restored from the persisted session) for a non-chat run that has them, using `completed = rs.status == RunStatusCompleted`. Gated on having nodes, so plain workflow runs keep BUG-170's no-seed behavior.
- `apps/local-runner/internal/runner/flow_step_runtime_test.go`: added `TestReconstructWorkflowRunRestoresStepTimeline` and `TestReconstructNonCompletedFlowRunRestoresPendingSteps`.

## Verification

- New reconstruct tests pass; BUG-170's `TestResumeRunReconstructs*` still pass (no-nodes path unchanged); broad flow/step/resume subset → 337 passed. `go build ./internal/runner/` clean.
- Not verified live (no runner + provider account) — flagged in `BUG-178` (`V-3`).

## Notes

- No persistence schema change: reuses `ActiveFlowNodes`, already persisted for edge-driven routing after restart (BUG-NOTE-CP42 #16). Display-only restore — does not re-engage the executor on resume.
- Exact per-step status for a non-completed resumed flow is approximate (all PENDING); persisting the step snapshot for full fidelity is a deferred enhancement. The reported completed-run case is restored exactly (all DONE).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-178
change_type: bugfix
summary: Rebuild a flow run's step-runtime timeline from persisted flow nodes on resume so a completed flow-mode history run shows its steps after a server restart instead of "No step-runtime data"
# --->8---
