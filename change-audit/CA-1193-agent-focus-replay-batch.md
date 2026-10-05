# CA-1193 — focusing a coder/agent run replayed its backlog per-event and froze the timeline

## Evidence (live CP-03 / run-225691)

Opening a coder child that accumulated thousands of events stuttered hard:
`focusAgentRun` → `client.streamRun(runId, 0)` replays the whole persisted
history, and `consumeAgentStream` applied every frame through **three**
`set()` calls (activity stamp, `applyEvent`, replay-cursor write). A 120-event
backlog produced 491 store updates in the repro test; real coder legs carry
orders of magnitude more.

## Root cause

`consumeHistoryReplayStream` (main-run reopen) already batches the persisted
backlog into a single `applyHistoryReplayEvents` flush bounded by
`handle.lastEventSeq` (the "lazy load" fix). `consumeAgentStream` — the
focused-child path — never got that treatment: same replay, per-event renders.

## Fix

- `consumeAgentStream` gains a `lastEventSeq` param and mirrors the history
  path: frames with `seq <= lastEventSeq` buffer and apply in one
  `applyHistoryReplayEvents` pass; the terminal stop uses the shared
  `shouldStopHistoryReplay`; only events past the durable boundary (live tail)
  keep the per-event path. Old runners without `lastEventSeq` fall back to
  the previous behavior unchanged.
- Both call sites wired: `focusAgentRun` passes `handle.lastEventSeq`;
  `backToMainRun` passes the snapshot's `lastEventSeq` or the resumed
  handle's.
- Render bound per operator request: `TIMELINE_PAGE_SIZE` 6 → 3 prompts.

## Regression

`store.focusAgentRunReplay.test.ts` (new): 120-event backlog lands in ≤20
store updates (was ~491) while still assembling the timeline; live tail past
the boundary still applies per-event. Existing focus/replay suites
(`store.test`, `store.timelineWindow`, `store.run75035-timeline-isolation`,
`store.spectator`) green; the six `store.test` failures observed are the
pre-existing baseline (identical with the change stashed).
