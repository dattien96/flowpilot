# CA-1204 — Owner-debate legs get the missing-verdict reprompt (live run-183756, run-174243)

## Evidence
Ledger fingerprint `grok-owner-truncated|run-183756|debate_trigger`
(high): owner_1/owner_2 legs (grok, verdict_only posture) ended their turns
mid-investigation — last_message was intent-prose ("locating owner def / SS
slice / TDD signatures"), no submit_review_outcome call, no verdict, no
violating paths. Twice in run-183756; same pattern 4x in run-174243.
debate_synthesis then had nothing to synthesize, submitted blocked, and the
flow escalated WAITING_USER_APPROVAL.

## Root cause
`settleFlowChildTurnCompletedLocked` already reprompts cohort children that
complete without a recorded machine verdict — but the gate enumerates which
parent flows require verdicts: `flowRequiresSynthesisMachineVerdict`
(acceptance_nodes contains "synthesis") and `flowRequiresHubMachineVerdict`
for plan_synthesis/synthesis/cp_synthesis. The vibe-owner-debate overlay
declares `acceptance_nodes: []` and `debate_synthesis` is not in
`hubInboundCohortName`, so owner legs — the exact cohort whose whole output
IS the verdict — were never covered.

## Fix
`vibeOwnerDebateMemberRequiresVerdictLocked(parent, child)` — true when the
parent's mounted topology is the owner-debate graph and the child's
stepID/label resolves to a node with `cohort: owner_debate`. Added to the
existing reprompt condition, so owner legs get the same bounded (2) reprompt
with the ordering-explicit submit_review_outcome instruction, the same
reinvokeInFlight/verdictRepromptInFlight guards, and the same post-cap
verdict-less completion as review-cohort members.

## Scope
Record-only helpers and verdict gating unchanged — debate_synthesis
done-edge semantics untouched. Non-owner cohort members inside the overlay
are unaffected (guard test).

## Tests
`ca1204_owner_debate_verdict_reprompt_test.go` — owner leg verdict-less
completion reprompts (count=1, back to running, no verdict-less cohort
entry); a non-owner_debate cohort member in the same overlay skips reprompt.
