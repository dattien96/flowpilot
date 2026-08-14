---
id: CA-503
feature_key: cli-tui
title: /open shows flow name; hydrate agents for Tab /agent picker
date: 2026-08-14
status: COMPLETE
---

## Change

After /open of a flow run:

1. **Status label** uses catalog/builtin human name (`grok-flow`), not UUID /
   short id (`resolveFlowDisplayName`). Silent flow-list prefetch upgrades label
   when catalog was empty at open.
2. **Hydrate sub-agents** via `GET /client/workflow-runs/{id}/agents` so
   `/agent` `/agents` Tab picker, Tab cycle, and step `[open]` work after reopen
   (agentRuns was only filled from live `agent_graph` SSE).
3. TUI `AgentRunSummary.Label` + match label in resolve/focus/suggestions
   (display `my-reviewer` not only agent package name).

Does not undo CA-502 open ModeFlow, CA-501 seed filter, CA-499 step open.

## Provider impact

Case 1 (agnostic). HTTP agent list + catalog name only.

## Tests

New cases in `tui_open_flow_chrome_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: /open uses flow display name and hydrates agent list for /agent Tab picker
# --->8---
