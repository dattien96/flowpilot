# CA-1115 — Task-458: desktop step liveness indicator from SSE arrivals

## Why

Live run-69320 (PrivateVault Task-024): multi-minute coder/reviewer legs
rendered the exact same "running" row whether the leg was streaming events
or silently parked. The only liveness signal was tailing the runner ndjson
logs by hand — which is also how every stall in that session was actually
detected. The backend already streams everything needed; the desktop was
just not observing it.

## What changed

`apps/desktop-flowpilot/src/state/runActivity.ts` (new):

- `RUN_ACTIVITY_STALE_MS = 30_000` — the single staleness threshold (D-3),
  well under the runner's ~2m stall watchdog.
- `eventActivityRunId` — run identity per event type, mirroring
  `isEventForRun`: graph/bus events carry `parentRunId` on the payload,
  everything else uses `workflowRunId`.
- `stampRunActivity` — next `lastActivityByRun` map for one arrival;
  same-ref no-op when the event has no run identity.
- `activityCountsForStatus` — terminal lanes (`completed`/`failed`/
  `cancelled`) never contribute to the family rollup.
- `lastActivityAt`, `runActivityKind`, `formatActivityAge` — read-side
  derivation (max stamp, live/quiet split, compact age text).

`apps/desktop-flowpilot/src/state/store.ts`:

- `AppState.lastActivityByRun: Record<runId, epochMs>` — cleared on every
  run/session reset alongside `_runReplaySeq`.
- Stamped in all four stream consumers after staleness and seq-boundary
  checks: `consumeStream`, `consumeHistoryReplayStream` (live tail only —
  persisted backlog `continue`s past the stamp), `consumeAgentStream`
  (before the orchestration-type skip), and `consumeOrchestrationStream`
  (before the graph/bus vs timeline split, so focus-guarded main-run
  arrivals still count).

`apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx`:

- Chip next to the running-step name: `live — last event Ns ago` under the
  threshold, `quiet Nm` past it. Rendered only while a step is RUNNING —
  approval-parked and terminal steps show nothing (AC-3). Rolls up main
  run + non-terminal `agentRuns` legs.
- 1s `setInterval` tick ages the label; the interval exists only while the
  sidebar is visible AND a step is RUNNING — no global render loop (D-2).

`apps/desktop-flowpilot/src/styles.css`: `.flow-sidebar-live` /
`-live` / `-quiet` chip styles on the `--ok`/`--warn` tokens.

## Invariant

The indicator is a presence signal derived only from real SSE arrivals —
replayed backlog can never stamp freshness, and no log text reaches the
UI. No runner changes.

## Tests

`src/state/runActivity.test.ts` (9 cases): stamp routing for regular /
graph / bus events, same-run restamp, live→quiet flip exactly at 30s,
terminal-status exclusion, family rollup max, age formatting. The eight
pre-existing phase1 failures (history-replay ordering, selectProject
reset, …) reproduce identically on the clean baseline — unrelated.
