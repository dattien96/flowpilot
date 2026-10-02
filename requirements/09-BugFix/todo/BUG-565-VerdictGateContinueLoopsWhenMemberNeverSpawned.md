# BUG-565 — Continue on `missing_review_verdict` gate loops when the deficient member was never spawned

- **ID:** BUG-565
- **Severity:** Medium–High (park→Continue→instant re-park loop; same
  wedge class as BUG-561 on a different gate reason)
- **Status:** open
- **Found:** live run-69320 (Task-024 sprint), 2026-10-02 14:01–14:06

## Symptom

After a hub_stall park was resumed, the `synthesis` hub turn submitted
`done`, the verdict gate rejected it
(`synthesis done blocked — missing machine verdict from reviewer` —
correct fail-closed), the flow escalated and parked. A plain Continue
re-drove the same hub on identical state → identical `done` → identical
reject → re-park **16 seconds later**. Repeatable.

## Root cause

`resumeVerdictDeficientMembers` (review_done_verdict.go:351, CA-1098)
covers only "cohort member ran but verdict missing/not-approved" — it
maps deficient cohort labels onto **live children**. In this run the
`reviewer` leg had never been spawned (the chain was still pre-coder —
the sprint's actual position was tdd-approved → awaiting coder), so no
child carried the `reviewer` label → `labels` empty → returns false →
generic resume re-drives the escalated `synthesis` hub itself on
identical state → same `done` → same gate → park loop.

## Wedge chain that led here (context)

- BUG-562 reseed wiped `synthesis` to PENDING → the tdd leg's legitimate
  `done` was consumed as `flow_control_stale_hub_done` without dispatch
  → coder never spawned.
- BUG-564 stranded `pendingHubReinvoke` → watchdog parked the run.
- On resume the hub (prompt: "synthesize the current node outcome…
  must call submit_review_outcome") read the approved state and
  submitted `done` — premature because the chain hadn't advanced.

## Recovery that worked

`gate-decision` option `custom` with explicit instruction ("do NOT submit
done — dispatch the coder leg now") — the re-driven hub spawned the coder
leg correctly (child_spawn at 14:06:30).

## Second defect in the same window (14:28)

`advanceHubDoneThroughEdge`'s verdict gate escalated **while the reviewer
leg was in-flight** (spawned 14:28:30, verdict gate fired 14:28:53) →
escalate → `flow_parked_awaiting_user` cancelled the reviewer mid-turn
(`cohort_member_failed`) → the verdict could never arrive. This is the
BUG-560 kill pattern on `synthesis`: BUG-560/561 added a defer check to
`runAuditNode` (`hasOpenCohort`/`hasRunningSprintStep`), but the
synthesis hub-done verdict gate at `flow_control_rejected_missing_review_verdict`
has no equivalent in-flight check. The same defer must apply wherever a
missing-verdict gate can escalate while the member that would produce it
is still running.

(Continue on this second park DID work: `resumeVerdictDeficientMembers`
found the now-existing cancelled `reviewer` child and re-drove it —
`verdict_deficient_member_redrive` 14:31:01 → turn-84752 on run-84486.)

## Expected fix direction

When `isReviewVerdictGateReason` fires and `resumeVerdictDeficientMembers`
finds no live deficient member, the generic resume should NOT blind
re-drive the hub on identical state. Options:

- If the verdict-gated node's inbound cohort member was never dispatched,
  route Continue to dispatch THAT member's leg (the flow knows the node —
  `expected` labels exist, they're just unspawned), or
- Re-drive the hub with an injected note: "done rejected: <member>
  verdict missing — drive the missing leg first, do not finalize"
  (equivalent to what the custom gate-decision achieved manually), or
- At minimum fail Continue with a distinct code instead of looping.

Also note the interaction: BUG-561's sprint-evidence routing already
demonstrates the "escalated node → route Continue to hub with note"
pattern — this is the same shape for the verdict-gate family.
