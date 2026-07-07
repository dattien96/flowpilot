# CA-249: Normalize Stale Child Agent Status In Restart Disk Fallback

## What changed

- `apps/local-runner/internal/runner/interactive_service.go`: `listAgentRunSummaries`'s `SessionIndexReader` disk-fallback branch now wraps `session.Status`/`session.AgentStatus` in the pre-existing `normalizeResumedStatus` (previously only used by `reconstructRun`), so a child agent read straight from disk after a restart (nothing tracking it live) reports `cancelled` instead of a stale `running`/`starting`/`waiting_*`.
- Added test: `TestListAgentRunSummariesNormalizesStaleRunningStatusAfterRestart` (`interactive_service_test.go`).

## Why

Found live, immediately after fixing BUG-250 in the same CP-36 Scenario 5 pass: reopening a restarted flow's hub chat showed its reviewer children still `running` in the Agents panel, even though a live process listing confirmed no runner or provider CLI process existed anymore. See [BUG-251](../requirements/09-BugFix/done/BUG-251-Restarted-Child-Agent-Shows-Permanently-Stale-Running-Status.md).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-251
change_type: bugfix
summary: normalize a restarted child agent's stale running/starting/waiting status to cancelled in the disk-fallback read path
# --->8---
