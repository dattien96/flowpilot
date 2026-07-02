# CA-216: Revert Sub-Agent Approval Mirror To Hub Stream

## Summary

Reverts `BUG-177` / `CA-214` per user decision. Mirroring a sub-agent's `permission_required` onto the hub run's stream made concurrent cohort approvals visible on the main view, but in live use it flooded the main view with unresolved approvals ("2/3/14 approvals required" accumulating) and contributed to the main run appearing to hang. Sub-agent approvals now stay in the agent (child) view: the user focuses that agent to approve, or runs YOLO for a hands-off UX.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: removed the parent-stream mirror block in `turnBridge.RequestApproval`; a sub-agent's approval is emitted only on its own run stream again.
- `apps/desktop-flowpilot/src/state/timelineReducer.ts`: reverted the `permission_required` reducer to its original (no `approvalId` dedup — the dedup was only needed to guard the mirror's double-surface).
- `apps/local-runner/internal/runner/flow_step_runtime_test.go`: removed `TestChildApprovalMirroredToHubStream`.
- `requirements/09-BugFix/done/BUG-177-*.md`: status → `cancelled` with a revert note.

## Verification

- `go build ./internal/runner/` clean; desktop `npm run typecheck` clean; flow/approval subset (`go test -run 'Flow|Cohort|SubmitFlowControl|Reconstruct|Reseed|SpawnChild'`) → 161 passed (only pre-existing codex-resume binary failures remain). `TestSpawnChildRunWaitTrueWaitsThroughApprovalGate` (child approval gate) still passes.

## Notes

- The underlying #5 gap (concurrent cohort approvals not on main) is intentionally left unaddressed; the accepted behavior is per-agent approvals + YOLO.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-177
change_type: refactor
summary: Revert the sub-agent approval mirror to the hub stream (BUG-177); sub-agent approvals stay in the agent view, since the mirror flooded the main view with unresolved approvals
# --->8---
