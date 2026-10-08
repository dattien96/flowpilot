# BUG-657 — `Interrupt` writes a durable `run_stop` fence keyed on the leg's own run id, but every `releaseHubStopFenceForFollowUp` release keys `parentRunID`: a fenced leg can never be reprompted — every redrive dies with `turn cancelled before send (run stop fence)`

- **ID:** BUG-657
- **Severity:** High — interrupting a leg is a one-way door; there is no
  operator or engine path that clears the fence on the leg itself, so
  the leg is permanently dead weight stamped `running`/`cancelled`.
- **Status:** OPEN (captured live, run-523131, leg run-584652)

## Evidence chain (all live)

1. Operator `interrupt` on zombie leg `run-584652` wrote a durable
   `run_stop` fence (`dispatch.ndjson` rev 2, `stopped`) keyed to the
   leg id.
2. Follow-up adjudication `adj-55` correctly re-armed the leg's reprompt
   intent — the redrive turn (`turn-592023`) failed in <1s with
   `turn cancelled before send (run stop fence)`.
3. All release call sites (`interactive_service.go` ~10400,
   `dispatch_live.go`) pass `rs.parentRunID` — the parent fence, never
   the leg's own — so no follow-up can ever release a leg-level fence.

## Root cause

Fence keying asymmetry: interrupt fences `rs.id` (the interrupted run),
release scans `rs.parentRunID`. Leg-scoped stops therefore persist
forever; parent-scoped stops release on follow-up as designed.

## Fix direction

- `F-1` Release both keys on any authorized follow-up: check/release the
  fence on `rs.id` AND `rs.parentRunID` (whichever is armed).
- `F-2` Or store the fence once on the parent with the interrupted child
  id recorded — single key space, no orphan fences.

## Regression coverage

- `TestBug657_InterruptedLegRedrivesAfterFollowUp` — interrupt leg →
  adjudication reprompt → turn dispatches (no fence abort).
- `TestBug657_ParentFenceStillHonored` — parent stop fence still blocks
  until released (no security regression).
