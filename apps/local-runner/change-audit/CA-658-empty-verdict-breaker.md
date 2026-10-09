# CA-658 — empty-verdict circuit breaker (BUG-663)

## Scope
One block in `restoreVibeFlowAfterDebate` + messaging. Aborted owner legs
(tool-permission denials) resolve no verdict rows, so the BUG-652
signature comparison stays blind and every remount re-debates vacuously
(live run-641691: seven owner legs, one suite_regressed card, parent
timeline frozen).

## Fix
- Consecutive empty-verdict resolutions for the same gated entity
  escalate (human park, no reprompt) exactly like an identical repeat.
  The first empty resolution still proceeds — legitimate verdict-less
  restores (drift-only hub debates, no gated children) are untouched.
- Implemented as an `EMPTY` sentinel inside the existing durable
  per-entity ledger: no new persisted fields, no store/resume migration;
  the sentinel can never collide with a real `acid=verdict` signature.
  A real verdict after empty (or vice versa) proceeds — fail-safe
  direction: may miss exotic alternations, never false-parks new
  content. Escalate messaging names the empty-verdict breaker.
- Out of scope (noted, not changed): verdict_only legs receiving
  reconcile-work (the denial root cause), and the possibly stale
  governing spec (CP-11 card on a CP-12 run) — both need product
  decisions beyond this breaker.

## Tests
- `bug663_empty_verdict_breaker_test.go` (new): RED (second empty armed
  another reprompt) → GREEN. First-empty proceeds, second-consecutive
  escalates (no reprompt, loop blocked, reason names breaker),
  empty-then-real proceeds.
- All BUG-652 pins green (breaker semantics for real sigs unchanged).
- Debate/restore/remount sweep green except pre-existing
  `TestReconstructResumeRestoresDistinctReviewerStatusesByPersistedLabel`
  (fails identically on clean main — verified via stash).
- `detect-changes` gate before commit.
