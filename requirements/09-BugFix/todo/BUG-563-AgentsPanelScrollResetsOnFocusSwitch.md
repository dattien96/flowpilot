# BUG-563 — Agents board scroll jumps to top when switching focused agent

- **ID:** BUG-563
- **Severity:** Low–Medium (UX annoyance; with 4+ running agents the
  clicked agent scrolls out of view immediately)
- **Status:** open
- **Found:** live run-69320 watch session, 2026-10-02 ~13:25–13:39

## Symptom

In the desktop "Agents" board (`AgentsPanel.tsx`), clicking a running
agent to focus it (`focusAgentRun(run.runId)`, ~lines 278/338) resets the
panel list's scroll position to the top. With several agents running the
just-clicked row scrolls out of view. Operator report: "vu chuyển agent
bị scroll lên top".

## Notes for the fix

- Same class as BUG-558 (chat timeline scroll restore) but a different
  scroll container — the BUG-558 anchor-restore covered the main timeline
  only; the agents list needs its own preserve-on-rerender (or stop the
  remount that resets `scrollTop`).
- Suspect paths: `focusAgentRun` → store navigation → panel re-render with
  a new list identity / remount, or the list reorders (running-first sort)
  on the focus change and scroll anchoring is lost.
- Verify whether the scroll reset also fires on `agent_graph_updated`
  refreshes while the operator is mid-scroll.

## Repro

1. Run a flow with ≥4 concurrent children (debate cohorts reproduce this
   naturally).
2. Scroll the agents board, click a lower agent → list jumps to top.
