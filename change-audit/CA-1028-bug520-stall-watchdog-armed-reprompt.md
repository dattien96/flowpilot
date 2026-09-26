# CA-1028 — BUG-520: hub stall watchdog counts armed gate-reprompt intents as child activity

Date: 2026-09-26 — BUG-520 found live on run-60145 (round-5 tournament drive)

## Change

`apps/local-runner/internal/runner/hub_stall.go` — `hasActiveFlowChild`:

- New `repromptArmed` predicate: `pendingGateRepromptPrompt != "" ||
  pendingGateRepromptStepID != ""`. A child with an armed gate-reprompt
  counts as active and cannot be classified as a ghost.
- `pendingFlowGateSettle` intentionally left out: BUG-354 contract —
  stale settle without a live gate cancel must not shield the hub. The
  armed-reprompt signal is safe: durable intents carry gen + lease +
  permanent-fail budget, so a wedged reprompt cannot shield forever.

## Why

The stall watchdog's whole job is distinguishing "no progress" from
"progress the hub can't see". An armed reprompt is unambiguous pending
work — the child WILL get another turn. Missing it parked a healthy
tournament mid-cohort (live run-60145), and `parkFlowForAwaitingUser`
then wiped the intent, leaving the child parked forever.

## Tests

- `TestBug520_StallDoesNotFireOnArmedGateReprompt` — verified RED
  pre-fix (watchdog parked the hub), GREEN after.
- `TestBug520_TrueGhostChildStillStalls` — real ghosts still stall.

## Risk

Low (GitNexus: 3 direct callers, LOW). `shouldParkHubWriteTurn` and
`notifyTurnIdle` gain the same — correct — signal: a child with a queued
reprompt must park hub writes and must count as non-idle.
