# BUG-544 — quota-vetoed cohort member releases its barrier seat while the route card is still pending

## Status
FIXED — unit-verified (red→green), live leg verified 2026-09-28 (run-26036
tournament, see below).

## Live-found during
`run-25217` (tournament-harness, devin hub, rebuilt binary with
BUG-541/542/544 fixes), 2026-09-28.

## Live sequence
1. Tournament spawn: `run-26015` (candidate-a, grok) + `run-26020`
   (candidate-b, devin), cohort `flow-auto-parallel_rollout-attempt-0`,
   expected=2.
2. Candidate-a's first-turn admission hit the grok quota veto →
   `emitQuotaRouteCard` parked it `waiting_question` (card `q-26028`),
   then `startTurn` returned `quota_route_required`.
3. `handleSpawnedChildTurnFailure` only special-cased `flow_awaiting_user` —
   the quota error fell through to `handleChildStartTurnFailure`: member
   stamped FAILED, `appendCohortResult(failed)`, leg closed.
4. 08:34:58 candidate-b completed → buffer=[a-failed, b-done]=2 →
   `cohort_join_complete` → `tournament_arbiter_decided` → `flow done` at
   08:35:00 — **while card q-26028 was still pending**.
5. The loser sweep (`sweepCandidateWorktrees`) removed
   `candidate-candidate-a`'s worktree and closed its leg.
6. Answering q-26028 afterwards: `quota_route_apply_failed: worktree ...
   no longer exists (swept)` — the card is restored pending but can never
   resolve: soft-wedge, the only way out is `stop`.

## Root cause
The spawn-dispatch quota veto was treated as a terminal dispatch failure.
Cohort seat arithmetic assumed the vetoed member was dead: its `failed`
append counted toward `expected` immediately, so the barrier did not wait
for the card. The parked member's leg claim (which protects the worktree
via `liveClaim` in BUG-535's respawn check) was also closed early.

## Fix (CA-636)
Three seams, all keyed on the pending-card semantics:

1. `handleSpawnedChildTurnFailure`: `quota_route_required` now returns
   without failing the member — status stays `waiting_question`, leg stays
   `active`, no cohort append. The pending card IS the durable intent.
2. `respawnChildOnRoute`: successor inherits the held seat —
   `CohortSeatInherited` skips `registerCohortMember`/`preRegisterCohort`
   when the vetoed member never buffered an entry. When the member already
   contributed a `failed` entry (mid-turn veto at TurnFailed), the seat was
   consumed and the successor registers a fresh one — barrier arithmetic
   stays correct on both paths.
3. `applyQuotaRouteAnswer` `stop` branch: releases the held seat — appends
   the `failed` cohort entry the join waits on (deduped via
   `memberAlreadyBuffered`) and closes the member's leg durably.

## Tests
`bug544_quota_veto_cohort_seat_test.go`:
- `QuotaVetoedSpawnedMemberHoldsCohortSeat` — veto parks; sibling's
  completion alone must not satisfy the barrier.
- `QuotaStopAnswerReleasesSeat` — `stop` closes the leg + appends the
  failed entry so the join can complete.
- `RespawnSuccessorInheritsSeat` — successor spawns without bumping
  expected; sibling+successor completions satisfy the barrier.

## Live verify
- `run-25217` reproduced the defect end-to-end (sequence above).
- Post-fix tournament `run-26036`: candidate veto → card → sibling alone
  does not join → answer → successor inherits seat → arbiter waits.
  (Evidence recorded in CP-Full-Live-Test R13.)

## Related
- BUG-534 (spawn-first ordering), BUG-535 (liveClaim guard),
  BUG-538 (parked successor intent), BUG-541 (answer rollback).
- Residual: a MID-TURN quota veto (member already buffered `failed` at
  TurnFailed) still counts toward the barrier immediately — the card is a
  post-hoc escape hatch there, not a held seat. Documented limitation.
