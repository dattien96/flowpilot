# CA-929 — Desktop: single transient heartbeat failure killed the app

## Summary

Manual CP-83 verification: mid-run (~14:06) the desktop app closed itself
while the runner stayed alive. Runner log shows the client count dropping
to zero, then normal `orphaned_work → idle_grace → draining_shutdown` — the
runner did not crash; the desktop quit first.

## Root cause

`heartbeatOnce()` called `runnerLost()` (notify + `ports.quit()` →
`app.quit()`) on **any** heartbeat failure. A burst of large ACP payloads
(~72KB records at 14:06:22–23) stalled one heartbeat past the 4s call
timeout → single transport error → app suicide. Runner-side evidence:
lease expiry at 14:06:36 (~15s TTL after the last good beat at ~14:06:21),
no `/release` call — an abrupt client exit, not a graceful quit.

## Fix

`heartbeatRequest()` wraps the heartbeat call in a bounded retry:
3 attempts, 400ms spacing (`heartbeatRetryMs` port override for tests).
3 attempts ≈ max ~12.8s — still inside the 15s lease TTL, so a recovering
runner keeps seeing a live client.

Preserved semantics:
- `LifecycleError` (answered HTTP: `lease_unknown`, `stale_generation`,
  `invalid_lease_token`) is authoritative — thrown immediately, never
  retried; the existing re-register path handles it.
- `draining_restart` → reconnect path unchanged; planned restarts
  unaffected.
- Sustained transport failure still notifies + quits
  (`UnplannedRunnerLossClosesWithNotice` passes unmodified).
- The retry sleep uses a real ref'd `setTimeout`, not
  `ports.setTimeoutFn` — an injected/parked scheduler would leave
  `heartbeatOnce()` pending forever.
- `quitting` guards: the loop bails and `heartbeatOnce()` skips
  `runnerLost()` if the user already initiated close — no spurious
  "runner stopped unexpectedly" during intentional shutdown.
- Beat scheduling unchanged: `tick` reschedules in `.finally()`, so beats
  stay serial — no overlap even though a worst-case beat now exceeds the
  5s cadence.

## Tests (additive)

`src/lifecycle/desktopLifecycle.retry.test.ts`:
- one transient failure then success → no quit, no notice, lease kept
- sustained failure → all attempts exhausted → notify + quit
- failure then recovery inside one beat → same leaseId, no re-register

## Files

- `apps/desktop-flowpilot/src/lifecycle/desktopLifecycle.ts`
- `apps/desktop-flowpilot/src/lifecycle/desktopLifecycle.retry.test.ts` (new)

## Notes

GitNexus MCP was unreachable during this change (`Failed to list tools for
server gitnexus`); impact/detect-changes gates could not run — flagged per
repo rules.

# ---8<--- flowpilot:change-ledger
feature_key: runner-lifecycle
source_doc_id: CP-83
change_type: bugfix
summary: heartbeat retries transient transport failures in-beat before runnerLost; lease errors and quit paths unchanged
# --->8---
