# BUG-628 — owner_1 (grok) never produces a machine verdict; cohort re-drives on a single stance until deadlock

- **ID:** BUG-628
- **Severity:** Medium (every debate round wasted legs + stalled the
  resolution into round-cap deadlocks; contributed to the run-174243
  escalate)
- **Status:** open
- **Found:** live run-174243 (Task-033 sprint), 2026-10-03 ~23:08–23:43

## Symptom

Across every owner-debate round on run-174243 (rounds 0–3, 8+ owner
legs total — including the BUG-624 duplicated pair):

- `owner_1` (grok/grok-4.7) legs completed their turns but **never
  emitted a `submit_review_outcome` verdict** — the synthesis note
  records: *"owner_1 (grok) never returned a stance across all rounds"*
  and *"two owner-agent fragments narrate intent but render no verdict"*.
- `owner_2` did eventually produce a stance which governed each round.
- The debate loop kept `continue`-ing (openIssues=3) partly waiting for
  the missing stance, then escalated/deadlocked → tournament rescue.

## Defect

The verdict machinery tolerated a permanently-silent owner: each round
re-spawned both owners, round 0's no-verdict completion even triggered
the `cohort_member_verdict_reprompt` path once, yet the debate proceeded
to verdicts decided by half the cohort — and the rounds burned full
provider turns chasing a verdict that structurally never arrives.

Open question (needs one focused reproduction): does the grok adapter
actually surface `submit_review_outcome` to the owner legs — tool
registration, schema mismatch, or does the grok leg simply narrate and
end turn? `owner_2` (also grok) did emit, so the failure is not uniform
across grok legs — may be prompt/context dependent (owner_1 may have
drawn the poisoned branch: the stale `tdd-signatures.md` Task-033 tail
block).

## Expected fix direction

- Reproduce on a single grok owner leg: confirm whether
  `submit_review_outcome` reached the leg's tool list and whether its
  output parsed; fix whichever stage silently drops it.
- Bound the "silent member" cost: a member that completes N times with
  no verdict should mark the cohort **degraded** (verdict proceeds from
  the members that did answer, flagged in the join note) rather than
  re-driving whole rounds for it.
- Provider-parity: verify the verdict tool surface on grok explicitly —
  this is exactly the class of per-provider gap the parity rule exists
  for.
