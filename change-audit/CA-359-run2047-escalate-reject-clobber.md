# CA-359 — run-2047: escalate form overwritten then hub_stalled

## What the user saw

Review Loop looked “normal” (coder + reviewers progressed, models Grok correct)
but main never `done` — after ~2m a **Needs your decision** card said:

> hub has made no progress for 2m0s (no turn, gate, or reinvoke in flight)

That is **F-0 hub_stalled**, not a provider timeout.

## Actual timeline (feature log)

1. Round 0: coder DONE → reviewers DONE → `hub_reinvoke_scheduled` (synthesis).
2. Hub called **continue** → round 1 coder + new reviewers.
3. Hub then **escalate** (gate / review summary) → `flow_parked_awaiting_user`
   cancelled in-flight round-1 reviewers.
4. Cancelled children emitted TurnFailed → `transition("rejected")` **overwrote**
   `loop.Status=blocked` (escalate card state).
5. Reinvoke after cohort fail was deferred; loop no longer “blocked”.
6. After 2m idle, F-0 fired **hub_stalled** (rejected was not treated as parked).

So: flow was **not** a clean happy path to done; synthesis chose continue/escalate,
and a status clobber turned a valid escalate pause into a fake “hub timeout”.

## Fix

- `transition("rejected")` does not clobber `blocked|stopped|done|paused`.
- F-0 does not fire over `rejected`/`approved` either.

## Tests

- `TestTransitionRejectedDoesNotClobberBlocked`
- `TestTransitionRejectedStillWorksWhenRunning`
- `TestHubStallDoesNotFireWhenBlocked`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Do not clobber escalate blocked with rejected after park-cancel; prevent hub_stalled over parked loops
# --->8---
