# BUG-672 — Armed Reinvoke Intent Never Dispatches Once Fail Counter Exceeds Cap

## Metadata

- Document ID: `BUG-672`
- Title: `pendingHubReinvoke consumed-while-blocked / cap-saturated — user Continue leaves armed intent undrained forever`
- Phase: `bugfix`
- Status: `FIXED — CA-1238`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Parent Documents: `BUG-667` (transient retry classes), `hub_stall` sweep
- Tags: `reinvoke, continue, stall, severity:high`

## Evidence (live run-752742)

- `hub_parked` reinvoke failures burned `hubReinvokeStartFailCount` past the
  watchdog's `<=3` drain bound.
- After the loop was unblocked, an operator Continue returned a loop snapshot
  but the armed `pendingHubReinvoke` was never consumed or dispatched —
  the hub sat permanently until a hand-injected turn.

## Root Cause

- `resumeFlowWithFeedback`'s non-blocked path redrove quietly without
  checking the armed pending intent; the watchdog only drains while
  `hubReinvokeStartFailCount <= 3` — a saturated counter means nobody ever
  dispatches the armed intent.
- Automatic retry budget and *operator* Continue are conflated: the cap must
  bound unattended retries, not a human explicitly pressing Continue.

## Fix

- In `resumeFlowWithFeedback`, both branches: when `pendingHubReinvoke` is
  armed, consume it, preserve its prompt, reset `hubReinvokeStartFailCount`,
  and dispatch `maybeAutoReinvokeHub{,WithPrompt}` — including when the loop
  is not technically `blocked` (rejected/partially-healed states included).

## Regression Test

- `bug672_reinvoke_intent_drain_test.go`
  `TestBug672_ContinueDrainsArmedReinvokeOnNonBlockedLoop`
