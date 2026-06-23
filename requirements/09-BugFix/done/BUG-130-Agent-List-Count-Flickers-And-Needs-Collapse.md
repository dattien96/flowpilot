# BUG-130: Agent List Count Flickers And Needs Collapse

## Metadata

- Document ID: `BUG-130`
- Title: `Agent List Count Flickers And Needs Collapse`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/todo/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [CA-120: Agent spawn UX hardening](../../../change-audit/CA-120-agent-spawn-ux-hardening.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, agents-panel, ui, race-condition`

## AI Quick View

### Summary

- The Agents panel list sometimes refreshes and shows an incorrect agent count, and grows unbounded with no way to collapse it.
- Root cause of the count flicker: `agentRuns` is written by both the orchestration SSE stream and a fire-and-forget `refreshAgentRuns()` HTTP fetch; a stale fetch can land after a fresher SSE snapshot and overwrite it, momentarily changing the count. There is no de-dup by run id and no sort.
- Fix: add a stale-response guard (`_agentRunsLoadSeq`) so a late fetch cannot overwrite a newer SSE update; de-dup the rendered list by run id and sort newest-first; collapse the active and closed lists to the latest 5 with a "show more" toggle.

### Current Ask

- Stop the agent count from flickering on refresh, show the latest agents on top, and collapse the list when it exceeds 5 items.

### Key Decisions

- `V-1` `refreshAgentRuns` bumps and checks a sequence so its result is discarded if a newer SSE `agent_graph_updated` (which also bumps the sequence) arrived meanwhile.
- `V-2` The panel de-dups `agentRuns` by `runId` and sorts by `createdAt` descending before rendering and counting.
- `V-3` Active and closed lists render at most 5 items by default, with a toggle to expand/collapse the remainder.

### Constraints

- Keep SSE as the authoritative live source; the HTTP fetch is only a backfill and must never clobber a newer snapshot.
- Do not change the backend agent summary contract.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` — `refreshAgentRuns`, `applyEvent`/`applyOrchestrationEvent` `agent_graph_updated` branches, `_agentRunsLoadSeq`.
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — list derivation, count, collapsible rendering.

## 1. Issue Summary

The Agents panel displays a "N running" count and lists child agents. Because `agentRuns` is set by two independent sources — the orchestration SSE stream and a fire-and-forget `refreshAgentRuns()` fetch — a slow fetch returning a smaller/older set can overwrite a fresher SSE snapshot, so the visible count briefly drops or jumps. The list also had no sort (order was nondeterministic) and no collapse, so it could grow long.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) (P-7 Agents panel)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) (§4 desktop Agents panel)
- impacted system spec: [SS-06](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Environment and Reproduction

- environment: FlowPilot desktop, a session with several spawned agents, normal SSE + periodic refresh.
- reproduction steps:
  1. Spawn several agents so SSE updates the panel.
  2. Trigger a `refreshAgentRuns()` (e.g. send a prompt / re-focus) while SSE is also updating.
  3. Observe the count momentarily shows an incorrect number, and the list order is unstable.
- frequency: intermittent (race-dependent); list-length/sort issues are deterministic.

## 4. Expected vs Actual

- expected: a stable, de-duplicated count; latest agents on top; a collapsed list (max 5) that expands on demand.
- actual: flickering count from stale-fetch overwrite, no sort, unbounded list.

## 5. Impact

- users affected: anyone using the Agents panel with more than a couple of agents.
- workflows affected: agent monitoring/navigation.
- severity: Medium — misleading count and cluttered list, no data loss.

## 6. Root Cause

- confirmed cause: `refreshAgentRuns()` unconditionally `set({ agentRuns })` with no stale guard, racing the SSE `agent_graph_updated` handler that also sets `agentRuns`; the render had no de-dup or sort.
- evidence: store has `_historyLoadSeq`/`_remoteHistoryLoadSeq` guards for analogous fetches but none for agent runs; both SSE branches set `agentRuns` directly.

## 7. Fix Strategy

- `F-1` Add `_agentRunsLoadSeq`; `refreshAgentRuns` increments it before the fetch and only applies the result if the seq still matches.
- `F-2` Both `agent_graph_updated` branches bump `_agentRunsLoadSeq` so an in-flight fetch is invalidated by a newer SSE snapshot.
- `F-3` In `AgentsPanel`, de-dup `agentRuns` by `runId` and sort by `createdAt` descending; derive count and lists from that.
- `F-4` Collapse active and closed lists to 5 items with a `▾ Show N more` / `▴ Show fewer` toggle.

## 8. Validation

- `V-1` `npx tsc --noEmit` (desktop) — pass (no type errors).
- `V-2` Manual: with multiple agents, the count no longer flickers on refresh; the newest agent is on top; lists collapse at 5 and expand/collapse via the toggle.
- `V-3` Unit test harness: the desktop vitest suite currently fails to load on this machine with a pre-existing ESM config error (`require is not defined in ES module scope`) affecting all five suites (including files untouched by this change), so component unit tests could not be executed here; type-checking is green.

## 9. Regression Guard

- tests: type-check guards the store/panel signatures; the seq-guard mirrors the existing `_historyLoadSeq` pattern.
- alerts: a flickering count returning indicates the SSE seq-bump or fetch guard regressed.
- audit checks: any new writer of `agentRuns` must bump `_agentRunsLoadSeq`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this is a UI-correctness delta to the Agents panel.
- notes left unchanged on purpose: the backend agent summary contract and SSE event shape are unchanged.
