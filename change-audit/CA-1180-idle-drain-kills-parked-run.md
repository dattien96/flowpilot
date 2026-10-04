# CA-1180 — idle-grace drain terminalizes a live sprint (work=0 false read + unconditional stop-all)

## Evidence (live run-204891, CP-03 Task-039 sprint, 2026-10-05)

```
00:06:15  scaffold child run-205821 turn_completed (reprompt turn)
00:06:19→00:07:00  post-turn gate ran native_host_tests.sh (PASS)
00:07:00  [vibe-gate] debate-routed reprompt budget exhausted attempts=2 — escalating
00:07:01  [lifecycle] phase=idle_grace clients=0 work=0        ← false read
00:07:15  [settle] finalized run=run-205821                   ← still working
00:07:32  draining_shutdown → stop-all → cancelled run-204891 + run-205821
00:07:33  drain complete — exiting
```

The run sat dead ~4h until manually restarted. Task-039 work products
survived uncommitted in the workspace; the sprint was recovered via
`agent-loop/continue` re-firing the durable boundary.

## Root cause — two stacked defects

**1. `LiveWorkSnapshot` under-counts engine work.** The inventory counted
only `turnInFlight`, `turnCancel`, `postTurnGateCancel`, `flowInlineCancel`,
and root loops in `running`/`paused`. Between gate-eval-done and
settle-finalize — and whenever owed dispositions are armed
(`pendingFlowGateSettle`, `pendingResume*`, `pendingGateReprompt*`) — a run
that is seconds from converging counts as zero work. The 30s `IdleGrace`
then fires `draining_shutdown` while the settle is still being finalized.

**2. `runSystemDrain` cannot tell housekeeping from a user stop.** Every
drain — confirmed /system action, signal, OR timer-driven idle-grace expiry
— ran `StopAllForSystemAction`, which terminalizes every non-terminal run.
A sprint parked `waiting_user_approval` with durable session/intent state
(the exact state the restart-reconstruction path is built to resume) was
converted into a permanent cancellation. `DrainRequester()` already
returned `""` for timer drains — the distinction existed but was never
plumbed to the drain executor.

## Fix

- `LiveWorkSnapshot`: armed owed dispositions (`pendingFlowGateSettle`,
  `pendingResume*` prompt/step, `pendingGateReprompt*` prompt/step) now
  count as protected `flow` work — gen counters deliberately ignored
  (high-water marks; the prompt fields are the armed state).
- `LifecycleDrainInput.Auto`: set when `DrainRequester()` is empty
  (timer-driven drains only — accepted drains record a real requester).
- `runSystemDrain`: `Auto` drains skip `StopAllForSystemAction` — a clean
  housekeeping exit; `cleanupSessionsBounded` still reclaims provider
  processes. Accepted drains keep the full confirmed stop-all.

Out of scope (flagged): manual `/system/restart` also stop-alls live work —
arguably wrong for a reconnect-grace restart, kept as-is pending design
review.

## Regression

`internal/runner/bug1180_lifecycle_idle_drain_test.go` — three red→green
shapes: armed `pendingFlowGateSettle`, armed `pendingResume*`
(the CA-1179 park shape), armed `pendingGateReprompt*` each count as work.
`internal/lifecycle`, `internal/cli`, `internal/runner` lifecycle/drain
filters, and `internal/tui/app` all green.
