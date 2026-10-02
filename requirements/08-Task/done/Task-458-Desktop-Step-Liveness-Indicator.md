# Task-458: Desktop Step Liveness Indicator ("actually alive" signal)

- Document ID: `Task-458`
- Title: `Surface a per-run/step liveness signal in the desktop UI so a "thinking" step is distinguishable from a silently stuck one`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-10-02`
- Last Updated: `2026-10-02`
- Parent Documents: ``
- Child Documents: ``
- Related Documents: `live run-60899/run-69320 watch sessions`
- Replaces: ``
- Tags: `desktop`, `ux`, `sse`, `liveness`, `observability`

## AI Quick View

### Summary

- While a flow step runs, the desktop shows a static "thinking" state —
  identical whether the leg is actively producing events or has been silent
  for ten minutes. Operators currently tail the Go runner ndjson logs to
  tell the difference.
- The backend already streams everything needed: every `ProviderEvent`
  (`message_delta`, `tool_started`, `tool_completed`, `token_usage_updated`,
  `file_changed`, `turn_*`, `agent_graph_updated`, …) reaches the desktop
  over SSE through `consumeOrchestrationStream` / the run event consumer.
- Fix is UI-side only: track `lastActivityAt` per run (and per step where
  resolvable) on every incoming SSE event and render a small "live Ns ago"
  / pulse indicator that goes visibly stale after a threshold.

### Current Ask

- A running step shows a real-time liveness cue (e.g. `● live — last event
  4s ago`, ticking) driven by actual SSE arrivals; when no event has
  arrived for > N seconds the cue degrades (e.g. `○ quiet 3m`) instead of
  looking identical to healthy streaming.
- No log text is rendered into the UI — this is a presence signal only.

### Key Decisions

- `D-1` No backend change: derive liveness from SSE event arrival
  timestamps in `store.ts` — the stream already carries per-run events;
  a heartbeat of real traffic is the honest signal.
- `D-2` Keep it subtle: text + dot on the running step row
  (`FlowTimelineSidebar` / step runtime rows), not a banner or log pane.
- `D-3` Staleness threshold in one constant (start ~30s); the runner's own
  stall watchdog (~2m) defines the outer bound — the indicator should go
  quiet well before the watchdog escalates.

### Open Questions

- None.

## Scope

- `apps/desktop-flowpilot/src/state/store.ts`: `lastActivityByRun: Record<runID, epochMs>`
  updated in the central event fan-in (single place all SSE events pass —
  `applyOrchestrationEvent` and the per-run consumer), cleared on run
  terminal status.
- Step-level attribution: optional v2 — run-level is enough for v1 since
  events carry `runId` and the focused run is what the operator watches.
- Component: small ticking label on the active step (needs a 1s interval
  re-render or `Date.now()` tick state — reuse whatever ticking pattern the
  codebase already has; do not introduce a global re-render loop).

## Acceptance Criteria

- `AC-1` A running flow step shows a live indicator whose freshness is
  driven by real SSE event arrivals.
- `AC-2` >30s with no events on an in-flight run visibly degrades the
  indicator (stale/quiet state), distinct from the active state.
- `AC-3` Terminal/parked runs show no fake liveness.
- `AC-4` No runner changes; no log text in the UI.
- `AC-5` Unit test: fake event stream → `lastActivityByRun` updates and
  staleness derivation flips at the threshold.

## Live motivation

- run-60899 / run-69320 (CP-02 vibe-tasks, 2026-10-02): during multi-minute
  coder/reviewer legs the UI was indistinguishable from a hang; the only
  way to confirm liveness was tailing
  `.flowpilot/logs/features/agent-flow-engine/<run>.ndjson`.

## 11. Completion Notes

- result: `lastActivityByRun` stamped on every live SSE arrival in the four
  stream consumers (`consumeStream`, `consumeHistoryReplayStream` live tail,
  `consumeAgentStream`, `consumeOrchestrationStream`); `runActivity.ts`
  carries the routing/derivation helpers; `FlowTimelineSidebar` renders a
  `live — last event Ns ago` / `quiet Nm` chip on the running step with a
  1s local tick that only runs while a step is RUNNING.
- stamping deliberately lives in the consumers, not `applyEvent`, so
  replayed backlog (`seq <= afterSeq` / persisted replay) cannot fake
  freshness. Terminal/parked runs show no indicator — read-side gating via
  step status plus `activityCountsForStatus` exclusion of terminal legs.
- run-level rollup (main + non-terminal legs) chosen over step-level for
  v1 per D-2/scope; child legs only stamp when their stream is focused or
  mirrored — honest signal, documented limitation.
- follow-ups: none.
- upstream docs updated: this file.
- audit: `change-audit/CA-1115-task458-run-liveness-indicator.md`
