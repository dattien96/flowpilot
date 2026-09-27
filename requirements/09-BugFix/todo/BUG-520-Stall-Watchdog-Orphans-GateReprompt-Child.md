# BUG-520 — Hub stall watchdog fires while a cohort child has an armed gate reprompt; park orphans the child

Status: FIXED + live-verified (r8/r9 tournament runs 69516/76075/82594: zero hub_stalled across every gate-reprompt window; r7 control run-60145 parked mid-cohort pre-fix)
Filed: 2026-09-26 (round-5 live test, run-60145)
CA: CA-1028

## Symptom (live run-60145)

Tournament run: candidate-a (grok) DONE 18:59:23; candidate-b (devin,
run-60174) turn completed 19:04:38 → its post-turn gate evaluated
(`child gate reprompt attempt=0`, armed reprompt intent gen=1) → at
19:04:39 the hub stall watchdog fired `hub_stalled` ("no progress for
2m0s") on the parent → `parkFlowForAwaitingUser` froze the cohort and
**wiped the child's armed `pendingGateRepromptPrompt`/Gen** → the
reprompt dispatch 409'd on `flow_awaiting_user` → child parked
`waiting_user_approval` with `pending_gate_code_paths` but no reprompt
intent durably → nothing ever re-drives it (durable record shows
`pending_gate_reprompt_prompt: null`).

Cascade observed on the same run:
1. Operator `continue` dispatched plain hub turns (codex) while the
   tournament join still waited on candidate-b's step
   (WAITING_USER_APPROVAL — never terminal).
2. The outer run status stamped `completed` while `candidate-b`,
   `tournament_arbiter`, `merge_and_audit` remained
   WAITING/PENDING — a dishonest terminal surface.
3. A second `continue` unblocked the loop (`running`) but the orphaned
   child still has no re-drive path — run is wedged.

## Root cause

`hasActiveFlowChild` (hub_stall.go) decides watchdog activity from:
in-flight turn, pending turn prompt, pending approval/question, live
gate cancel, Starting/Waiting statuses, non-ghost Running. It did **not**
count an armed `pendingGateRepromptPrompt`/`StepID` — a queued
remediation turn is literally pending child work. A child in the
gate-reprompt window (turn over, gate evaluated, reprompt armed,
dispatch pending) read as a ghost → watchdog parked the hub → park wiped
the intent → orphan.

## Fix (CA-1028)

`hasActiveFlowChild`: a child with armed `pendingGateRepromptPrompt` or
`pendingGateRepromptStepID` counts as active (and cannot be a ghost).
`pendingFlowGateSettle` deliberately still does not count alone —
BUG-354 contract: a stale settle without a live gate cancel must not
shield the hub; reprompt intents are bounded by the durable-intent
lease/fail-budget machinery.

## Tests

`internal/runner/bug520_stall_gate_reprompt_child_test.go` —
- `TestBug520_StallDoesNotFireOnArmedGateReprompt`: child running, turn
  done, gate cancel nil, reprompt armed → `checkAndBlockStalledHub` must
  NOT fire; `hasActiveFlowChild` true. (Verified RED before the fix —
  watchdog parked.)
- `TestBug520_TrueGhostChildStillStalls`: running child with nothing
  armed → watchdog still fires (BUG-354 ghost contract preserved).

Regression: hub-stall battery (`Run333`, `Run43831`, BUG-354 watchdog,
park-active-child, shouldParkHubWriteTurn) all green.

## Residual observations (not fixed here — follow-ups)

- **Orphan cure**: children already parked with wiped intents
  (`pending_gate_code_paths` armed, no reprompt prompt) have no re-drive
  path on parent unpark — needs a re-arm/re-dispatch sweep on resume.
- **False-terminal surface**: `continue` on a `hub_stalled` run let a
  plain hub turn settle `status=completed` while flow steps remained
  WAITING/PENDING — status honesty gap worth its own record.
- **Park clears child reprompt intents**: by design ("no turns behind
  the form") but there is no re-arm on unpark — see orphan cure.
