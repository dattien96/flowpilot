# BUG-132: Recently-Closed Agents Disappear While A New Agent Runs

## Metadata

- Document ID: `BUG-132`
- Title: `Recently-Closed Agents Disappear While A New Agent Runs`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-130: Agent List Count Flickers And Needs Collapse](./BUG-130-Agent-List-Count-Flickers-And-Needs-Collapse.md), [CA-121: Agent panel runtime fixes](../../../change-audit/CA-121-agent-panel-runtime-fixes.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, agents-panel, ui, regression, sse`

## AI Quick View

### Summary

- Regression from BUG-130: while a new agent is running, every item in the "Recently closed" section disappears (only "Running" shows); they reappear after the sub-agent finishes.
- Root cause: the BUG-130 `_agentRunsLoadSeq` guard made each SSE `agent_graph_updated` invalidate the in-flight HTTP `refreshAgentRuns` fetch. The SSE graph snapshot is in-memory only and omits disk-persisted closed children that the HTTP list (`listAgentRunSummaries`) includes, so while SSE fired (agent running) the panel was stuck on the leaner snapshot and dropped the closed entries.
- Fix: SSE updates now **merge** into the existing list by runId (incoming wins) instead of replacing, and no longer bump the fetch sequence. Closed children persist across live updates; the HTTP refresh remains the authoritative superset.

### Current Ask

- Keep "Recently closed" agents visible while a new agent is running.

### Key Decisions

- `V-1` The SSE `agent_graph_updated` handler merges its `runs` into the existing `agentRuns` by runId rather than replacing, so it cannot drop children it does not know about.
- `V-2` The SSE handler no longer bumps `_agentRunsLoadSeq`; the sequence now only guards overlapping HTTP refreshes.
- `V-3` `refreshAgentRuns` stays an authoritative replace (the superset for the parent), so chat switches still clear the previous parent's agents.

### Constraints

- Must not reintroduce the BUG-130 count flicker.
- Must still clear agents when switching to a different parent chat.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` — `applyEvent` / `applyOrchestrationEvent` `agent_graph_updated` branches, new `mergeAgentRunsById`, `refreshAgentRuns`.
- `apps/local-runner/internal/runner/interactive_service.go` — `listAgentRunSummaries` (HTTP superset incl. disk sessions).
- `apps/local-runner/internal/runner/agent_orchestrator.go` — `graphSnapshot` (in-memory only).

## 1. Issue Summary

The HTTP `listAgentRunSummaries` returns a superset — live runs + historical + all persisted provider sessions on disk (the closed children). The SSE `graphSnapshot` carries only the orchestrator's in-memory summaries. BUG-130 added a sequence guard that let SSE updates invalidate the in-flight HTTP fetch; so while an agent ran (continuous SSE), the panel showed only the in-memory snapshot and the disk-only closed children vanished, returning when SSE quieted and a fetch landed.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) (P-7 Agents panel)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) (§4)
- impacted system spec: [SS-06](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Environment and Reproduction

- environment: FlowPilot desktop, a session with at least one completed (closed) child and then a newly spawned running child.
- reproduction steps:
  1. Spawn and complete one or more agents (they appear under "Recently closed").
  2. Spawn a new agent so the orchestration SSE emits `agent_graph_updated` repeatedly.
  3. Observe "Recently closed" empties while the new agent runs.
  4. When the new agent finishes, the closed items reappear.
- frequency: always while a new agent is actively running.

## 4. Expected vs Actual

- expected: closed children stay listed under "Recently closed" regardless of whether another agent is running.
- actual: they disappeared during the run and returned afterward.

## 5. Impact

- users affected: anyone watching the Agents panel during back-to-back agent runs.
- workflows affected: agent monitoring/navigation.
- severity: Medium — misleading panel state, no data loss.

## 6. Root Cause

- confirmed cause: BUG-130's `_agentRunsLoadSeq` guard plus SSE bumping that sequence caused the richer HTTP fetch to be discarded whenever SSE was active; the SSE snapshot lacks disk-only closed children, so they dropped from view.

## 7. Fix Strategy

- `F-1` Add `mergeAgentRunsById(existing, incoming)` and use it in both `agent_graph_updated` branches so SSE merges (never drops) by runId.
- `F-2` Remove the `_agentRunsLoadSeq` bump from the SSE branches.
- `F-3` Keep `refreshAgentRuns` as the authoritative replace (superset) with its own overlapping-fetch guard.

## 8. Validation

- `V-1` `npx tsc --noEmit` (desktop) — pass.
- `V-2` Manual: with closed agents present, spawning a new agent no longer clears "Recently closed"; the new agent appears under "Running" and closed items remain.
- `V-3` Unit test harness: the desktop vitest suite fails to load on this machine with a pre-existing ESM config error (`require is not defined in ES module scope`, from `vite-plugin-electron` loaded via `vite.config.ts`) affecting all suites, so component unit tests could not be executed here; type-check is green.

## 9. Regression Guard

- tests: type-check guards the store signatures.
- alerts: closed agents vanishing during a run indicates the SSE path reverted to replace or re-bumps the fetch sequence.
- audit checks: SSE agent updates must merge, not replace.

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this is a UI-correctness delta refining BUG-130.
- notes left unchanged on purpose: the backend list/SSE contracts are unchanged; only the client merge strategy changed.
