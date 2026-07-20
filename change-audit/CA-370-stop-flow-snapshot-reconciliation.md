# CA-370: Stop flow snapshot reconciliation

## Summary

Reconciled saved desktop run snapshots with the terminal graph returned by `stopAgentLoop`, including the stopped graph embedded in a partial durable-checkpoint error. This prevents a child-focused Stop from leaving the cached main snapshot as `running`, while preserving already completed and failed children.

## Verification

- `npx tsx --tsconfig apps/desktop-flowpilot/tsconfig.json --test apps/desktop-flowpilot/src/state/stop_parent_snapshot_reconciliation.test.ts`: 2 passed.
- `go test ./internal/runner -run "TestStopAgentLoopSnapshotReportsChildCancelledSynchronously|TestInterruptParentCancelsRunningChildAgents|TestNonCohortEntryFailSettlesStepAndClearsHubGateSettle" -count=1`: 3 passed.
- `npm --prefix apps/desktop-flowpilot run build`: passed.
- `git diff --check`: passed (line-ending warnings only).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-292
change_type: bugfix
summary: Reconcile child-focused saved snapshots with a stopped flow graph so main cannot remain visibly active after Stop.
# --->8---
