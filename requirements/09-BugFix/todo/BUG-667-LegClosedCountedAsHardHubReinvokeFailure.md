# BUG-667 — `leg_closed` Counted As Hard Hub-Reinvoke Failure, Stranding Armed Pending Forever

## Metadata

- Document ID: `BUG-667`
- Title: `leg_closed reject saturates hubReinvokeStartFailCount; pending reinvoke armed forever`
- Phase: `bugfix`
- Status: `FIXED — CA-1237`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Last Updated: `2025-01-24`
- Parent Documents: `CP-87 (quota gate)`, `BUG-284`, `BUG-564`, `BUG-1188`
- Related Documents: `BUG-666` (zero-active-leg makes leg_closed reachable for long windows)
- Tags: `hub-reinvoke, stall-watchdog, leg-lifecycle, severity:high`

## AI Quick View

### Summary

- `maybeAutoReinvokeHub`'s failure classifier treats `hub_parked` and `flow_awaiting_user` as transient-busy (no budget charge, deferred re-fire — BUG-564/BUG-1188 fixes), but **`leg_closed` is not in the transient set**.
- A `leg_closed` reject means a leg rotation is mid-flight or the chat is legless (BUG-666) — both are *wait-and-retry* conditions, not permanent failures.
- After 3 `leg_closed` rejects the consecutive-fail budget is spent: `shouldDrain=false`, `reinvokeInFlight=false`, `pendingHubReinvoke` stays armed but the stall-tick drain at `hub_stall.go:496` also gates on `<=3`. Armed-but-never-dispatched = **permanent hub_stalled loop** until manual `/turns` injection.

### Current Ask

- Classify `leg_closed` as transient-busy for hub reinvoke (same treatment as `flow_awaiting_user`), keep the pending armed, and let the deferred drain + stall tick re-fire.

### Key Decisions

- `V-1` `leg_closed` on the hub run is definitionally transient: the relay target either appears within ticks (rotation in flight) or the operator repairs the chat — neither outcome is helped by burning the budget.
- `V-2` Budget still protects against genuinely-permanent failure classes; transient classes only defer.

### Source Refs

- live `run-752742`: `hub_reinvoke_start_failed` ×3+ with `error="this run's leg was superseded; resubmit on the chat's active leg"`; flow then parked `hub_stalled` repeatedly; only manual `/turns` injection (relay to successor leg) unblocked it.
- `interactive_service.go:5296-5310` (transient classifier), `hub_stall.go:496` (drain cap).

## 1. Issue Summary

While the chat's legs rotated (quota routing + BUG-666 legless window), the hub's scheduled reinvoke called `startTurn(parentRunID)` on the closed flow-owner leg. `startTurn` correctly refused with `leg_closed` (no active sibling at that instant). Each refuse incremented `hubReinvokeStartFailCount`; at count >3 every drain path gated off and the armed `pendingHubReinvoke` was never dispatched — the sprint sat in `hub_stalled` for hours.

## 3. Environment and Reproduction

- environment: flow run whose hub leg is closed mid-rotation (provider switch in flight or legless chat)
- reproduction: close hub leg with no active sibling → trigger `maybeAutoReinvokeHub` → startTurn returns `leg_closed` → counter++ each retry → after 3 retries pending armed forever.
- frequency: every leg-rotation window that coincides with an armed reinvoke

## 4. Expected vs Actual

- expected: reinvoke waits through the legless window and dispatches once a leg exists
- actual: budget burns in 3 ticks; armed pending never drains; permanent stall

## 5. Impact

- severity: high — converts a transient leg gap into an unrecoverable stall without manual injection

## 6. Root Cause

- confirmed cause: `leg_closed` absent from the transient set at `interactive_service.go:5296` (only `hub_parked`, `flow_awaiting_user` skip the counter; `turn_in_progress`, `gate_in_progress` count but mark transient). `leg_closed` hits the `else` → `hubReinvokeStartFailCount++` → `shouldDrain = count <= 3`.
- evidence: live events `hub_reinvoke_start_failed` with the leg-superseded message; stall loop observed.

## 7. Fix Strategy

- `F-1` Add `leg_closed` (and its sibling `quota_route_committed` when it means "relayed to successor") to the transient-busy arm: do not increment `hubReinvokeStartFailCount`, keep `pendingHubReinvoke` armed, mark `transientBusy` so the deferred drain re-fires.
- `F-2` Reproducing test (additive): armed reinvoke + closed hub leg → classifier must not increment the fail counter and must re-fire on next drain tick.
