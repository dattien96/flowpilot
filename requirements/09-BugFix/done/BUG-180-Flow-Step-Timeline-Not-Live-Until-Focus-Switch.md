# BUG-180: Flow Step Timeline Does Not Update Live (Only On Focus Switch)

## Metadata

- Document ID: `BUG-180`
- Title: `Flow Step Timeline Does Not Update Live (Only On Focus Switch)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-175-Flow-Step-Timeline-Sidebar-Lost-Whenever-Main-Run-Not-Running.md`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, flow-mode, step-runtime, live-update`

## AI Quick View

### Summary

- The Flow-mode step timeline did not update as nodes progressed (coder → reviewers → synthesis); it only refreshed when the user switched focus between agents, which re-triggered a fetch.
- Root cause: the flow executor's step transitions (BUG-174) are `workflowStore.ApplyStepTransition` store writes with no dedicated event; the desktop only fetches `workflowStepRuntime` via `GET .../steps-runtime`, and `FlowTimelineSidebar` fetches it only in a `useEffect` keyed on `[visible, mainRunId]` — never on backend progress.
- Fix: every step transition rides alongside an `agent_graph_updated` event (child spawn/complete), which the hub's always-on orchestration stream already consumes. Trigger `refreshWorkflowStepRuntime()` there so the timeline updates live.

### Current Ask

- The step timeline must reflect node progress live, without a focus switch.

### Key Decisions

- `V-1` `consumeOrchestrationStream` calls `refreshWorkflowStepRuntime()` after applying an `agent_graph_updated` event. That helper self-guards (chatMode check, `_workflowStepRuntimeLoadSeq` stale-response guard, target-run match), so the extra fetches are safe and de-duped.

### Constraints

- Desktop-only; reuses the existing agent-graph correlation and the existing steps-runtime endpoint — no new backend event.
- No change to non-flow runs (the refresh no-ops for `chatMode !== "workflow_step_auto"`).

### Open Questions

- A dedicated `workflow_step_updated` backend event would be more direct but larger; the client-side hook is minimal and reuses the existing lifecycle correlation. Left as a possible future refinement.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts:1989-1990` (`consumeOrchestrationStream` — applied `agent_graph_updated` without refreshing step runtime)
- `apps/desktop-flowpilot/src/state/store.ts:495-513` (`refreshWorkflowStepRuntime` — self-guarding fetch)
- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx` (fetch only in a `useEffect` on `[visible, mainRunId]`)
- `apps/local-runner/internal/runner/flow_step_runtime.go` (`setFlowStepStatus` → `ApplyStepTransition`, store write only; no event)

## 1. Issue Summary

During a live flow run the step timeline stayed stale (e.g. all "pending", or a step stuck) until the user clicked into an agent, which re-fetched the step runtime. Node progress was invisible in real time.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local runner; a live Flow-mode run.
- reproduction steps:
  1. Start a flow run and watch the step-timeline sidebar.
  2. Let the coder finish and the reviewers spawn.
  3. Observe the timeline does not advance until you switch focus between agents.
- frequency: deterministic.

## 4. Expected vs Actual

- expected: the timeline advances live as nodes transition.
- actual: it updates only on a focus switch.

## 5. Impact

- users affected: anyone watching a live Flow-mode run.
- workflows affected: Flow Mode UI only; cosmetic but misleading (looks stuck).
- severity: medium — the primary progress surface appears frozen mid-run.

## 6. Root Cause

- confirmed cause: step transitions are store writes (`setFlowStepStatus` → `ApplyStepTransition`, `flow_step_runtime.go`) with no event; the desktop reads step runtime only via the `steps-runtime` endpoint, fetched by `FlowTimelineSidebar`'s `useEffect([visible, mainRunId])` — so nothing re-fetches on backend progress. `consumeOrchestrationStream` handled `agent_graph_updated` (child lifecycle) but did not refresh the step runtime (`store.ts:1989-1990`).

## 7. Fix Strategy

- `F-1` In `consumeOrchestrationStream`, after applying an `agent_graph_updated` event, call `void get().refreshWorkflowStepRuntime()` (`store.ts`). Since each executor step transition is emitted alongside an `agent_graph_updated`, the timeline now refreshes live on every node spawn/complete. `refreshWorkflowStepRuntime` self-guards, so the added fetches are safe and de-duped.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Not executed live: the timeline only renders in a live flow run (needs the runner + a provider account). The change reuses the existing self-guarding fetch on an event the stream already consumes; low risk. The user should confirm the timeline now advances without a focus switch.

## 9. Regression Guard

- tests: none (no render-test harness for this component; the desktop `vitest` suite cannot run in this environment — pre-existing ESM config error); typecheck is the gate.
- alerts: none.
- audit checks: recorded in `change-audit/CA-218-flow-step-timeline-live-refresh.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: a dedicated backend `workflow_step_updated` event is a possible future refinement (see Open Questions).
