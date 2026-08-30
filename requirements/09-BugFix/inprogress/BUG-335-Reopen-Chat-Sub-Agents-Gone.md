# BUG-335: Reopening a chat shows no sub-agents — agent hydrate only ran for flow opens

## Metadata

- Document ID: `BUG-335`
- Title: `Reopening a chat shows no sub-agents — agent hydrate only ran for flow opens`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-30`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md) (section F follow-up), [BUG-334](BUG-334-Opencode-Spawn-Agent-MCP-Connection-Closed.md)
- Child Documents: `none`
- Related Documents: BUG-251 (listAgentRunSummaries disk fallback), Task-082 (historical children)
- Replaces: `none`
- Tags: `tui, agents, hydrate, restart, severity-medium`

## AI Quick View

### Summary

- Operator report: after closing the TUI and reopening the same chat, `/agents` no longer listed the sub-agents spawned in the previous session.
- The TUI hydrates `m.agentRuns` from `cmdHydrateAgentRuns` (GET agent runs for the parent) — but the open handler dispatched it ONLY when `kind == "flow"`. Chat opens never fetched the graph, so a restart (which empties the in-memory state) left the agents panel and the new sidebar agents section empty.
- The server side already survives restarts: `listAgentRunSummaries` merges the BUG-251 disk path (`ListAllProviderSessions` filtered by `ParentRunID`) with live + historical children (proven by `TestAgentTreeSurvivesRunnerRestartBeforeSync`).

### Fix

- The `/open` handler now dispatches `cmdHydrateAgentRuns` for EVERY open (chat included); the existing in-flight guard + retry cap keep it cheap. Flow-only one-shot steps fetch is unchanged.
- A durable-rehydrate fallback inside `agentGraphSnapshot`/`listAgentRunSummaries` was implemented and then REVERTED during verification: seeding historical children from the store at any empty-graph snapshot polluted legitimate mid-lifecycle empty states (restore tombstone staging, flow child teardown) — 12+ restore/flow regressions, reverted to HEAD. The BUG-251 disk path in `listAgentRunSummaries` is the single durable fallback and already covers the restart scenario.

## Validation

- Red-checked end-to-end: with the fix reverted (`git show HEAD:` swap), the restart scenario fails; with `TestAgentTreeSurvivesRunnerRestartBeforeSync` (listAgentRunSummaries after restart) green on HEAD, the hydrate-on-chat-open change restores `/agents` + the sidebar section after restart.
- Full runner package after revert = committed HEAD state; remaining failures pre-existing/environmental (stash/isolation-verified). TUI package: the 7 documented pre-existing failures.

## Operator verify

Rebuild → reopen the chat that spawned `helper` → `/agents` and the sidebar `agents` section list the completed child again.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-57
change_type: bugfix
summary: Hydrate agent runs on every chat open so /agents and the sidebar agents section survive TUI restarts
# --->8---
