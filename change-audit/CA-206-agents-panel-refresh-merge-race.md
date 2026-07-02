# CA-206: Agents Panel Refresh Merge Race

## Summary

Fixed `BUG-169`: a spawned sub-agent could be narrated by the orchestrator but never appear in the Agents panel. Tracing ruled out a server-side registration/attribution defect (a new test proves workflow-mode-parent spawns list correctly, same as chat-mode) and found a real desktop-side race instead: the Agents panel's mount-time HTTP refresh replaced `agentRuns` wholesale, so a slow response landing after a live SSE update already added a freshly-spawned child could wipe it back out.

## What Changed

- `apps/desktop-flowpilot/src/state/store.ts`: `refreshAgentRuns` now merges its HTTP result into existing state via `mergeAgentRunsById` instead of replacing wholesale, matching the merge semantics BUG-132 already established for the SSE `agent_graph_updated` path.
- `apps/local-runner/internal/runner/agent_orchestrator_test.go`: added `TestListAgentRunSummariesHTTPWorkflowModeParent`, closing a test-coverage gap (all prior `listAgentRunSummaries` coverage used a `normal_chat` parent only) and formally ruling out a runKind-specific attribution bug.

## Verification

- `TestListAgentRunSummariesHTTPWorkflowModeParent` — passes.
- Full runner `go test ./...` — no new failures (two pre-existing, unrelated failures confirmed present without this change: `TestSkillsMerge*WithPrecedence`, `TestStartInteractiveAuthLaunchesFromWorkspace`).
- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live against the original repro — flagged in `BUG-169` (`V-4`); a genuinely-failed spawn narrated as success by the model remains a possible, separate, non-code-fixable explanation if the symptom recurs.

## Notes

- Investigated alongside `BUG-168` and `BUG-170` from the same user report.

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: BUG-169
change_type: bugfix
summary: Merge (not replace) the Agents panel's HTTP refresh result into existing state to close a race with live SSE agent-graph updates, and add workflow-mode-parent test coverage for spawn listing
# --->8---
