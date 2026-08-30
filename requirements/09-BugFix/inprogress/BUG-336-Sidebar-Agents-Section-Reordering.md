# BUG-336: Sidebar agents section flickered and re-ordered continuously

## Metadata

- Document ID: `BUG-336`
- Title: `Sidebar agents section flickered and re-ordered continuously`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-30`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md) (section F/K UX follow-up)
- Child Documents: `none`
- Related Documents: [BUG-335](BUG-335-Reopen-Chat-Sub-Agents-Gone.md) (section introduction)
- Replaces: `none`
- Tags: `tui, sidebar, agents, ordering, severity-low`

## AI Quick View

### Summary

- Operator report (flow mode): the sidebar `agents` rows jumped around and re-ordered continuously while the flow ran; screenshot showed main after two reviewers in arbitrary order.
- `m.agentRuns` is replaced by TWO delivery channels with different orderings — live `agent_graph_updated` snapshots (orchestrator children order) and `/open` hydrate polls (`ListAgentRuns`, live map + historical merge) — so every refresh could present the same runs in a different sequence.

### Fix

- `agentRunsSectionLines` sorts a copy of the rows into a DETERMINISTIC order before rendering: the main run (role/name "main") pins to the top, the rest sort by `CreatedAt` (spawn time, RFC3339 — lexicographic order is chronological), `RunID` as the tiebreak. Identical run sets now always render identically regardless of delivery order, which also lets the CA-633 frame cache serve identical frames instead of repainting (flicker).
- `client.AgentRunSummary` gains the `CreatedAt` field the runner already serialized.
- Steps section ordering untouched; spinner animation on RUNNING rows is the only remaining per-tick change (same as steps).

## Validation

- `TestAgentsSidebar_StableMainFirstThenSpawnOrder` (additive): shuffled delivery renders main → test_signatures → implement → reviewer by spawn time; a second service with the SAME set in a DIFFERENT delivery order renders byte-identical rows.
- Existing agents-sidebar tests (visible rows / hidden when empty / height cap) stay green; full tui package shows exactly the 7 documented pre-existing failures.

## UX follow-up (operator): agents section is chat-only; flow names agents on step rows

Flow mode already renders every agent-backed step in the steps view, so a
second agents list duplicated it. The sidebar `agents` section now renders in
CHAT MODE ONLY (`mode == ModeChat && !launch.IsCatalogWorkflow()`), and in
flow mode each step executed by a spawned agent carries an inline
` · agent: <name>` chip in the agent hue (`styleStatusAgent`) right on its
step row (via the existing `childRunForStep` match; main runs excluded by
that matcher). Tests: flow-mode sidebar hides the section; the agent-backed
step names its agent exactly once while agent-less steps stay bare.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-57
change_type: bugfix
summary: Sort the sidebar agents section deterministically (main first, then spawn time) so rows stop re-ordering between snapshots
# --->8---
