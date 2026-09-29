# BUG-552 — escalate park swallows the gate-decision routing record; routed decision never delivers

## Status
RESOLVED — CA-1067 (2026-09-29). Regression tests in
`apps/local-runner/internal/runner/bug552553_park_decision_and_cohort_test.go`
verified RED→GREEN. Filed from live run-15525 verification
(post CA-1062/1063/1064).

## Live-found during
Post-fix live verification on `:4421` (runner v3, `/tmp/fp-vibe-item2`
store root), 2026-09-29 ~13:41–13:45.

## Symptom
After the owner-debate synthesis escalated (owners voted
`suggest-requirement-change` — the scaffold-red contract contradiction),
the flow parked on `flow_parked_awaiting_user` with a gate-decision card.

The operator submitted `gate-decision` with option
`suggest-requirement-change`. The API accepted it and the response
reported `routed_to=run-15525` with the note that "resume carries the
decision". But the decision never reached a turn: the park kept
re-arming, and every subsequent `continue` re-escalated instead of
delivering the routed choice to the flow. The routing record existed but
was swallowed by the park path — no turn ever consumed it.

Expected: once an operator answers a gate-decision card, the routed
decision must be delivered to the parked flow exactly once and must
survive any number of subsequent park/continue cycles until delivered.

## Root cause (observed, not yet root-caused in code)
Escalate park (`flow_parked_awaiting_user`) cancels in-flight turns and
drops auto-intents. The gate-decision routing record rides the resume
path, so when the loop re-parks or the resume lands while the park is
being re-armed, the pending decision is dropped alongside the cancelled
turn's intents — same class of loss as BUG-551's orphaned
`pendingAgentContext`, but on the operator-decision channel rather than
the cohort-note channel.

The accept path ACKs routing (`routed_to`) before delivery is proven —
a three-outcome violation: the record is claimed delivered at ACK time
but no terminal/retryable/uncertain state is actually established for
the routed decision itself.

## Reproduction
1. Owner-debate (or any flow) escalates → `flow_parked_awaiting_user`
   with a gate-decision card whose options include
   `suggest-requirement-change`.
2. POST `gate-decision` with that option → 200, `routed_to=<root>`.
3. `continue` the root → the flow re-parks/re-escalates; the routed
   decision is never presented to any turn.

## Fix direction (not implemented)
- Make gate-decision delivery a durable intent on the parent (same
  model as pendingResumePrompt / pendingGateRepromptPrompt) so it
  survives park cycles, restarts, and cap-blocked reinvokes until a hub
  turn actually consumes it.
- ACK routing only after the durable intent is claimed — not before —
  so `routed_to` is a true record of accepted work.
- Regression test shape: escalate park → submit decision → re-park →
  continue → decision delivered exactly once to the next hub turn.
